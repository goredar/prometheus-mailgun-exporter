# prometheus-mailgun-exporter

Prometheus Mailgun Exporter

## Build

`make` will build both binary and Docker image

## Run

The exporter will serve metrics on `http://<ip>:9616/metrics`, and a healthz
endpoint on `http://<ip>:9616/healthz` for use in Kubernetes

```
export MG_API_KEY=<api_key>
export MG_DOMAIN=<domain1(,domain2,domain3,...)>
```

### Docker

```bash
docker run -ti --rm \
  --name prometheus-mailgun-exporter \
  -e MG_API_KEY=<key> \
  -e MG_DOMAIN=<domain|domain_list> \
  goredar/prometheus-mailgun-exporter:latest
```

### Binary

```bash
./prometheus-mailgun-exporter
```

## Europe Users

`export API_BASE=https://api.eu.mailgun.net/v4`

## Dashboard

The Grafana dashboard can be found
[here](https://grafana.com/grafana/dashboards/10663)
