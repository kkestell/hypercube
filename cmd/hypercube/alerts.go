package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/session"
)

// watchAlerts passes the runtime's alerts to the desktop app until ctx ends.
// A failed alert is reported and the watch goes on.
func watchAlerts(ctx context.Context, rt *session.Runtime, d *desktop.Desktop) {
	err := rt.WatchAlerts(ctx, func(alert session.Alert) {
		if err := sendAlert(d, alert); err != nil {
			fmt.Fprintln(os.Stderr, "hypercube: alerting the desktop app:", err)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: watching alerts:", err)
	}
}

// sendAlert passes an alert to the desktop app as the notification
// preferences allow. An unreadable config is reported and sends the alert as
// the defaults do.
func sendAlert(d *desktop.Desktop, alert session.Alert) error {
	n := config.DefaultNotifications
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: reading notification preferences:", err)
	} else {
		n = cfg.Notifications
	}
	wanted := n.Replies
	if strings.HasPrefix(alert.Key, "request:") {
		wanted = n.Requests
	}
	if !wanted || !n.Desktop {
		return nil
	}
	return d.Alert(alert, n.Sound)
}
