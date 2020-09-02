# Мониторинг доставки сообщений

## Установка

1. Создать тестовую команду и отправить @tadabot команды `/newbot alice` и `/newbot bob`

2. Создать конфигурационный файл `/etc/tdcheck/default.yml` вида 

```
listen: 127.0.0.1:8789
servers:
  - host: myserver.tada.team
    api_ping_interval: 30s
    ws_ping_interval: 10s
    check_message_interval: 30s
    test_team: xxxx
    alice_token: xxxx
    bob_token: xxxx
```

...где `host` — адрес вашей инсталляции tada, `test_team` — uid тестовой команды, `alice_token` и `bob_token` — токены, полученные на первом шаге.

3. Установить на сервер мониторинг, например, так: 
```
sudo -H -u www-data go get -u github.com/tada-team/tdcheck
```

4. Поставить мониторинг в автозапуск любым удобным способом, например через supervisor: 
```
# /etc/supervisor/conf.d/tdcheck.conf
[program:tdcheck]
command=/var/www/go/bin/tdcheck
autorestart=true
user=www-data```

5. Прописать сбор метрики в /etc/prometheus/prometheus.yml
```
  - job_name: tdcheck_my_server
    metrics_path: 'my.server'
    static_configs:
      - targets: ['localhost:8789']
```

6. Настроить отображение метрик `tdcheck_*` в Grafana
