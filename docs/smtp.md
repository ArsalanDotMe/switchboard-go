# SMTP notifications

Switchboard Go can send email when it switches away from an exhausted upstream
key and when all upstream keys have been exhausted.

Notifications are disabled unless `SMTP_HOST`, `SMTP_FROM`, and `SMTP_TO` are
all set.

Example with STARTTLS:

```bash
export SMTP_HOST="smtp.example.com"
export SMTP_PORT="587"
export SMTP_USERNAME="alerts@example.com"
export SMTP_PASSWORD="your-smtp-password"
export SMTP_FROM="alerts@example.com"
export SMTP_TO="you@example.com"
export SMTP_STARTTLS="true"
```

Equivalent YAML:

```yaml
smtp:
  host: "smtp.example.com"
  port: 587
  username: "alerts@example.com"
  password: "your-smtp-password"
  from: "alerts@example.com"
  to: "you@example.com"
  tls: false
  starttls: true
```

SMTP sending is asynchronous and best effort. Notification failures are logged
but do not fail client requests.
