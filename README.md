# Evolved

Standalone application for updating Evolution PIM badge with number of unread emails.

It has no GUI and the only purpose of it - to run as user daemon, check for Evolution files changes, get unread messages count from it's database and send dbus event with new count, so you'll get exact number of unread messages in all your mailboxes on task bar.

## Github notice

Github is a mirror. Please go to [my Gitea](https://code.pztrn.name/apps/evolved) for bug reporting, merge requests, etc.

## Dependencies

Golang 1.21+ and C compiler present in system and found in `PATH`, as Evolution database is in sqlite3.

## Installation

Just:

```text
go install go.dev.pztrn.name/evolved@latest
```

## Using

### Via systemd

Put this in `~/.config/systemd/user/evolved.service`:

```ini
[Unit]
Description=Evolution badge count.

[Service]
Type=simple
ExecStart=/path/to/evolved

[Install]
WantedBy=default.target
```
Replace `/path/to/evolved` for real path to binary.

After that do:

```text
systemctl --user daemon-reload
systemctl --user enable evolved.service
systemctl --user start evolved.service
```

### Via i3/sway/etc. configuration file for autostart

For i3 and sway there is `exec` thing that launches commands on startup. Feel free to use this.

Look for equivalent thing for other WMs.

## Caveat about desktop file name

Evolved using `org.gnome.Evolution.desktop` as launcher file name by default. Your distribution might name it differently, check `/usr/share/applications` or `~/.local/share/applications` for actual file name and pass `-desktop-file` parameter to Evolved binary.
