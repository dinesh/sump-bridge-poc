### How to build

    go build .

### How to run

    ./sumo-bridge -addr=<sumo-collector-url>
    Starting bridge
    bridge: sumo.go:121: collected metrics:  1
    
    metrics:
    # HELP bridge_counter The total number of processed events
    # TYPE bridge_counter counter
    bridge_counter 12

    ...

### How it works

It create a promethues registry and register all required metrics on it. Then it creates a background process to submit those metrics to Sumo collector periodically. The bridge is located at `pkg/bridge/sumo.go`.

### Things to do

I can see the metrics are being pushed to Sumo but not visible in the Sumo UI. Maybe not getting ingested correctly or getting filtered via collector rules. Something need to investigate.

