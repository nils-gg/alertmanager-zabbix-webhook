[![Build Status](https://travis-ci.org/gmauleon/alertmanager-zabbix-webhook.svg?branch=master)](https://travis-ci.org/gmauleon/alertmanager-zabbix-webhook)

## alertmanager-zabbix-webhook

Webhook that sends alerts to a Zabbix server via trapper items.  
You can create your Zabbix Hosts/Items/Triggers yourself - which gets tedious fast - or you can use https://github.com/gmauleon/alertmanager-zabbix-provisioner to pull the list of items and alerts directly from Prometheus and its Alertmanager.

However, if your Prometheus Alertmanager is not (automatically) reachable from your alertmanager-zabbix-webhook location, or you have no place to pull from Prometheus AND push to Zabbix server at the same time, or you have multiple remote Prometheuses and/or Alertmanagers, then you might use the Zabbix template that uses Low-Level-Discovery rules and automatically learns and populates your Zabbix targets for you, based on the alerts that it receives via this webhook: [P_Prometheus.xml](https://github.com/infonl/alertmanager-zabbix-webhook/blob/master/contrib/P_Prometheus.xml).

## Howto

Have a look at the default [config.yaml](https://github.com/infonl/alertmanager-zabbix-webhook/blob/master/config.yaml) for the possible parameters  
Kubernetes deployment manifests are in contrib/kubernetes
