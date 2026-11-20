# Metrics

`/metrics` gives the status of runs in the text format Prometheus reads, for Grafana to show and alert on. Every UI serves it, whatever parties it acts as, from the runs whose status the store lets it read, and it reads nothing inside a run. When the store cannot be read, a scrape fails, so Prometheus records the failure.

## The series

Every series is a gauge: a count or a time as the store holds it when Prometheus asks. An ended run leaves the store after its definition's `keep`, so a count of ended runs can fall, which a counter may not.

```
corbie_runs                           runs, running or ended and still kept, by status, failure, type, machine and location
corbie_waiting                        what running runs wait on (jobs on runners, timers, tickets), by type, machine and location
corbie_oldest_running_seconds         how long the oldest running run of each machine at each location has run
corbie_last_ended_timestamp_seconds   when a run of each machine at each location last ended, by status
```

Every run is counted, child runs included, at the location it runs at: a child at its parent's. A run's labels and its key are not used, so there is one series for each machine at each location, whatever its runs carry.

```
corbie_runs{status="running",type="statechart",machine="thermostat",location="/home/rooms/thermostat"} 3
corbie_runs{status="failed",failure="denied",type="flow",machine="fetch-title",location="/web/fetch-title"} 1
corbie_waiting{on="tickets",type="statechart",machine="review",location="/docs/review"} 1
corbie_oldest_running_seconds{type="statechart",machine="boiler",location="/home/devices/boiler"} 293400
corbie_last_ended_timestamp_seconds{status="done",type="flow",machine="report",location="/work/report"} 1760486580
```

## Asking

For Prometheus on another computer, a UI started as no party listens on every address, with its token kept in a file:

```
corbie ui none --listen :8373 --token-file /etc/corbie/ui-token
```

Prometheus shows the token as `Authorization: Bearer`, and its own interval sets how often it asks:

```
scrape_configs:
  - job_name: corbie
    authorization: { credentials_file: /etc/corbie/ui-token }
    static_configs: [ { targets: ["ops-1:8373"] } ]
```

The answer comes from the list of runs the UI already reads (see Serving), so a scrape reads nothing more from the store.
