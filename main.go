// Prometheus Mailgun Exporter
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mailgun/mailgun-go/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	namespace = "mailgun"
)

// Exporter collects metrics from Mailgun's via their API.
type Exporter struct {
	domains              []string
	APIKey               string
	APIBase              string
	scrapeStart          time.Time
	up                   *prometheus.Desc
	acceptedTotal        *prometheus.Desc
	clickedTotal         *prometheus.Desc
	complainedTotal      *prometheus.Desc
	deliveredTotal       *prometheus.Desc
	failedPermanentTotal *prometheus.Desc
	failedTemporaryTotal *prometheus.Desc
	openedTotal          *prometheus.Desc
	storedTotal          *prometheus.Desc
	unsubscribedTotal    *prometheus.Desc
	state                *prometheus.Desc
}

func prometheusDomainStatsDesc(metric string, help string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName(
			namespace,
			"domain_stats",
			fmt.Sprintf("%s_total", metric),
		),
		help,
		[]string{"name"},
		nil,
	)
}

func prometheusDomainStatsTypeDesc(metric string, help string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName(
			namespace,
			"domain_stats",
			fmt.Sprintf("%s_total", metric),
		),
		help,
		[]string{"name", "type"},
		nil,
	)
}

// NewExporter returns an initialized exporter.
func NewExporter() *Exporter {
	scrapeDomains := os.Getenv("SCRAPE_DOMAINS")
	if scrapeDomains == "" {
		log.Fatal().Msg("required environment variable SCRAPE_DOMAINS not defined")
	}

	apiKey := os.Getenv("MG_API_KEY")
	if apiKey == "" {
		log.Fatal().Msg("required environment variable MG_API_KEY not defined")
	}

	return &Exporter{
		domains:     strings.Split(scrapeDomains, ","),
		APIKey:      apiKey,
		APIBase:     os.Getenv("API_BASE"),
		scrapeStart: time.Now().UTC(),
		up: prometheus.NewDesc(
			prometheus.BuildFQName(
				"mailgun",
				"",
				"up",
			),
			"'1' if the last scrape of Mailgun's API was successful, '0' otherwise.",
			nil,
			nil,
		),
		acceptedTotal: prometheusDomainStatsTypeDesc(
			"accepted",
			"Mailgun accepted the request for incoming/outgoing to send/forward the email and the message has been placed in queue.",
		),
		clickedTotal: prometheusDomainStatsDesc(
			"clicked",
			"The email recipient clicked on a link in the email.",
		),
		complainedTotal: prometheusDomainStatsDesc(
			"complained",
			"The email recipient clicked on the spam complaint button within their email client.",
		),
		deliveredTotal: prometheusDomainStatsTypeDesc(
			"delivered",
			"Mailgun sent the email via HTTP or SMTP and it was accepted by the recipient email server.",
		),
		failedPermanentTotal: prometheusDomainStatsTypeDesc(
			"failed_permanent",
			"All permanently failed emails. Includes bounce, delayed bounce, suppress bounce, suppress complaint, suppress unsubscribe",
		),
		failedTemporaryTotal: prometheusDomainStatsTypeDesc(
			"failed_temporary",
			"All temporary failed emails due to ESP block, that will be retried",
		),
		openedTotal: prometheusDomainStatsDesc(
			"opened",
			"The email recipient opened the email and enabled image viewing.",
		),
		storedTotal: prometheusDomainStatsDesc(
			"stored",
			"The email recipient opened the email and enabled image viewing.",
		),
		unsubscribedTotal: prometheusDomainStatsDesc(
			"unsubscribed",
			"The email recipient clicked on the unsubscribe link.",
		),
		state: prometheus.NewDesc(
			prometheus.BuildFQName(
				namespace,
				"domain",
				"state",
			),
			"Is the domain active (1) or disabled (0)",
			[]string{"name"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.acceptedTotal
	ch <- e.clickedTotal
	ch <- e.complainedTotal
	ch <- e.deliveredTotal
	ch <- e.failedPermanentTotal
	ch <- e.failedTemporaryTotal
	ch <- e.openedTotal
	ch <- e.storedTotal
	ch <- e.unsubscribedTotal
	ch <- e.state
}

// Collect implements prometheus.Collector. It only initiates a scrape of
// Collins if no scrape is currently ongoing. If a scrape of Collins is
// currently ongoing, Collect waits for it to end and then uses its result to
// collect the metrics.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	var scrapeOK float64 = 1

	for _, domain := range e.domains {
		stats, err := e.getStats(domain)
		if err != nil {
			ch <- prometheus.MustNewConstMetric(e.state, prometheus.GaugeValue, 0, domain)
			log.Error().Err(err)
			scrapeOK = 0

			continue
		}

		ch <- prometheus.MustNewConstMetric(e.state, prometheus.GaugeValue, 1, domain)

		acceptedTotalIncoming := float64(0)
		acceptedTotalOutgoing := float64(0)
		clickedTotal := float64(0)
		complainedTotal := float64(0)
		deliveredHTTPTotal := float64(0)
		deliveredSMTPTotal := float64(0)
		failedPermanentBounce := float64(0)
		failedPermanentDelayedBounce := float64(0)
		failedPermanentSuppressBounce := float64(0)
		failedPermanentSuppressComplaint := float64(0)
		failedPermanentSuppressUnsubscribe := float64(0)
		failedTemporaryEspblock := float64(0)
		openedTotal := float64(0)
		storedTotal := float64(0)
		unsubscribedTotal := float64(0)

		for _, stat := range stats {
			acceptedTotalIncoming += float64(stat.Accepted.Incoming)
			acceptedTotalOutgoing += float64(stat.Accepted.Outgoing)
			clickedTotal += float64(stat.Clicked.Total)
			complainedTotal += float64(stat.Complained.Total)
			complainedTotal += float64(stat.Complained.Total)
			deliveredHTTPTotal += float64(stat.Delivered.Http)
			deliveredSMTPTotal += float64(stat.Delivered.Smtp)
			failedPermanentBounce += float64(stat.Failed.Permanent.Bounce)
			failedPermanentDelayedBounce += float64(stat.Failed.Permanent.DelayedBounce)
			failedPermanentSuppressBounce += float64(stat.Failed.Permanent.SuppressBounce)
			failedPermanentSuppressComplaint += float64(stat.Failed.Permanent.SuppressComplaint)
			failedPermanentSuppressUnsubscribe += float64(stat.Failed.Permanent.SuppressUnsubscribe)
			failedTemporaryEspblock += float64(stat.Failed.Temporary.Espblock)
			openedTotal += float64(stat.Opened.Total)
			storedTotal += float64(stat.Stored.Total)
			unsubscribedTotal += float64(stat.Unsubscribed.Total)
		}

		// Begin Accepted Total
		ch <- prometheus.MustNewConstMetric(
			e.acceptedTotal,
			prometheus.CounterValue,
			acceptedTotalIncoming,
			domain, "incoming",
		)
		ch <- prometheus.MustNewConstMetric(
			e.acceptedTotal,
			prometheus.CounterValue,
			acceptedTotalOutgoing,
			domain, "outgoing",
		)
		// End Accepted Total

		ch <- prometheus.MustNewConstMetric(e.clickedTotal, prometheus.CounterValue, clickedTotal, domain)
		ch <- prometheus.MustNewConstMetric(e.complainedTotal, prometheus.CounterValue, complainedTotal, domain)

		// Begin Delivered Total
		ch <- prometheus.MustNewConstMetric(
			e.deliveredTotal,
			prometheus.CounterValue,
			deliveredHTTPTotal,
			domain, "http",
		)
		ch <- prometheus.MustNewConstMetric(
			e.deliveredTotal,
			prometheus.CounterValue,
			deliveredSMTPTotal,
			domain, "smtp",
		)
		// End Delivered Total

		// Begin Failed Permanent Total
		ch <- prometheus.MustNewConstMetric(
			e.failedPermanentTotal,
			prometheus.CounterValue,
			failedPermanentBounce,
			domain, "bounce",
		)
		ch <- prometheus.MustNewConstMetric(
			e.failedPermanentTotal,
			prometheus.CounterValue,
			failedPermanentDelayedBounce,
			domain, "delayed_bounce",
		)
		ch <- prometheus.MustNewConstMetric(
			e.failedPermanentTotal,
			prometheus.CounterValue,
			failedPermanentSuppressBounce,
			domain, "suppress_bounce",
		)
		ch <- prometheus.MustNewConstMetric(
			e.failedPermanentTotal,
			prometheus.CounterValue,
			failedPermanentSuppressComplaint,
			domain, "suppress_complaint",
		)
		ch <- prometheus.MustNewConstMetric(e.failedPermanentTotal, prometheus.CounterValue,
			failedPermanentSuppressUnsubscribe,
			domain, "suppress_unsubscribe",
		)
		// End Failed Permanent Total

		ch <- prometheus.MustNewConstMetric(
			e.failedTemporaryTotal,
			prometheus.CounterValue,
			failedTemporaryEspblock,
			domain, "esp_block",
		)

		ch <- prometheus.MustNewConstMetric(e.openedTotal, prometheus.CounterValue, openedTotal, domain)
		ch <- prometheus.MustNewConstMetric(e.storedTotal, prometheus.CounterValue, storedTotal, domain)
		ch <- prometheus.MustNewConstMetric(e.unsubscribedTotal, prometheus.CounterValue, unsubscribedTotal, domain)
	}

	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, scrapeOK)
}

func (e *Exporter) getStats(domain string) ([]mailgun.Stats, error) {
	mg := mailgun.NewMailgun(domain, e.APIKey)
	if e.APIBase != "" {
		mg.SetAPIBase(e.APIBase)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	return mg.GetStats(ctx, []string{
		"accepted", "clicked", "complained", "delivered", "failed", "opened", "stored", "unsubscribed",
	}, &mailgun.GetStatOptions{
		Resolution: mailgun.ResolutionHour,
		Start:      e.scrapeStart,
	})
}

func main() {
	var (
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").
				Default(":9616").
				String()
		metricsPath = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").
				Default("/metrics").
				String()
	)

	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	kingpin.Version(version.Print("prometheus-mailgun-exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	log.Info().Msgf("Starting Mailgun exporter %v", version.Info())
	log.Info().Msgf("Build context %v", version.BuildContext())

	prometheus.MustRegister(NewExporter())
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Mailgun Exporter</title></head>
            <body>
            <h1>Mailgun Exporter</h1>
            <p><a href='` + *metricsPath + `'>Metrics</a></p>
			<p><a href='/healthz'>Health</a></p>
            </body>
            </html>`))
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	log.Info().
		Msgf("Starting HTTP server on listen address %s and metric path %s", *listenAddress, *metricsPath)

	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatal().Err(err).Msgf("%v", err)
	}
}
