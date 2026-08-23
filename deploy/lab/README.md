# Disposable end-to-end lab

This lab starts StePanel, MySQL, Prometheus, and Grafana. It is designed for
local demonstrations and migration testing, not production hosting.

```sh
docker compose -f deploy/lab/docker-compose.yml up --build -d
bash deploy/lab/run.sh
```

Open:

- StePanel: <http://localhost:8080>
- Prometheus: <http://localhost:9090>
- Grafana: <http://localhost:3000>

To submit the generated archive, use the dashboard or the API with
`confirm=IMPORT`. The lab contains no real credentials or customer data;
destroy it with `docker compose -f deploy/lab/docker-compose.yml down -v`.
