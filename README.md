# tdcheck

Prometheus.yml
```
  - job_name: tdcheck_my_server
    metrics_path: 'my.server'
    static_configs:
      - targets: ['localhost:8789']
```
