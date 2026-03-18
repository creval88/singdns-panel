package config

const DefaultConfigTemplate = `{
  "listen": ":9999",
  "session_key": "change-me",
  "audit_log": "logs/audit.log",
  "auth": {
    "username": "admin",
    "password_hash": "$2a$10$mhgzMC./.jG65Pw8OhUo1Ocmw9UrwsatLMrk7Ii95Ag0DcCKcR1/a"
  },
  "panel_update": {
    "release_dir": "/opt/singdns-panel/updates",
    "upgrade_command": "",
    "base_url": "",
    "channel": "stable",
    "arch": "amd64"
  },
  "services": {
    "singbox": {
      "service_name": "sing-box",
      "config_path": "/etc/sing-box/config.json",
      "url_path": "/etc/sing-box/url.txt",
      "bin_path": "/usr/local/bin/sing-box",
      "ctl_path": "/usr/local/bin/sbctl.sh"
    },
    "mosdns": {
      "service_name": "mosdns",
      "ctl_path": "/usr/local/bin/mdctl.sh",
      "web_url": "http://10.0.0.8:9099/log"
    }
  }
}`
