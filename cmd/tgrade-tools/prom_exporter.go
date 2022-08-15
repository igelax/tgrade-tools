package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/spf13/cobra"
	tmlog "github.com/tendermint/tendermint/libs/log"
)

const flagPromListenPort = "prometheus-listen-address"

func StartPrometheusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exporter [comma separated addresses to watch]",
		Short: "Start a prometheus exporter to collect liquid and total account balances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)
			var addrs []sdk.AccAddress
			for _, v := range strings.Split(args[0], ",") {
				addr, err := sdk.AccAddressFromBech32(v)
				if err != nil {
					return sdkerrors.Wrapf(err, "address: %s", addr)
				}
				addrs = append(addrs, addr)
			}
			promReg := prometheus.NewRegistry()
			logger := tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout)).With("module", "prometheus_exporter")
			balanceCollector := NewBalanceCollector(logger, queryClient, addrs...)
			prometheus.WrapRegistererWith(prometheus.Labels{}, promReg).MustRegister(balanceCollector)

			promListenAddr, _ := cmd.Flags().GetString(flagPromListenPort)
			if promListenAddr == "" {
				return errors.New("empty prometheus listener address")
			}
			prom, stop := newProm(cmd.Context(), promListenAddr, promReg)
			defer stop() //nolint:errcheck
			logger.Info("Prometheus started", "listen", promListenAddr)
			return prom.ListenAndServe()
		},
	}
	flags.AddQueryFlagsToCmd(cmd)
	cmd.Flags().String(flagPromListenPort, ":8081", "Prometheus http listen port")
	return cmd
}

func newProm(ctx context.Context, listenAddress string, reg *prometheus.Registry) (*http.Server, func() error) {
	appName := "tgrade-tools"
	reg.MustRegister(version.NewCollector(strings.ReplaceAll(appName, "-", "_")))

	mux := http.NewServeMux()
	httpServer := http.Server{
		Addr:    listenAddress,
		Handler: mux,
	}
	mux.Handle("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	return &httpServer, func() error {
		ctx, done := context.WithTimeout(ctx, time.Second)
		defer done()
		return httpServer.Shutdown(ctx)
	}
}

var _ prometheus.Collector = &BalanceCollector{}

type BalanceCollector struct {
	logger             tmlog.Logger
	queryClient        types.QueryClient
	totalBalancesDesc  *prometheus.Desc
	liquidBalancesDesc *prometheus.Desc
	addresses          []sdk.AccAddress
}

func NewBalanceCollector(logger tmlog.Logger, queryClient types.QueryClient, addr ...sdk.AccAddress) prometheus.Collector {
	return &BalanceCollector{
		logger:      logger,
		queryClient: queryClient,
		addresses:   addr,
		totalBalancesDesc: prometheus.NewDesc(
			"total_balance",
			"The current total token amount in TGD",
			[]string{"account"}, nil,
		),
		liquidBalancesDesc: prometheus.NewDesc(
			"liquid_balance",
			"The current total token amount in TGD",
			[]string{"account"}, nil,
		),
	}
}

func (b *BalanceCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- b.totalBalancesDesc
}

const stakeTokenDenom = "utgd"

func (b *BalanceCollector) Collect(metrics chan<- prometheus.Metric) {
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	for _, addr := range b.addresses {
		spendableResp, err := b.queryClient.SpendableBalances(ctx, types.NewQuerySpendableBalancesRequest(addr, nil))
		if err != nil {
			b.logger.Error("failed to query spendable balance", "cause", err, "address", addr.String())
			continue
		}
		metrics <- prometheus.MustNewConstMetric(b.liquidBalancesDesc, prometheus.GaugeValue, float64(spendableResp.Balances.AmountOf(stakeTokenDenom).Int64()), addr.String())

		totalResp, err := b.queryClient.Balance(ctx, types.NewQueryBalanceRequest(addr, stakeTokenDenom))
		if err != nil {
			b.logger.Error("failed to query total tgd balance", "cause", err, "address", addr.String())
			continue
		}
		metrics <- prometheus.MustNewConstMetric(b.totalBalancesDesc, prometheus.GaugeValue, float64(totalResp.Balance.Amount.Int64()), addr.String())
	}
}
