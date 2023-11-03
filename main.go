package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

var (
	cliFlagIsDebug         = flag.Bool("debug", false, "Enable debug output.")
	cliFlagDesktopFileName = flag.String("desktop-file", "org.gnome.Evolution.desktop", "Desktop file name from /usr/share/applications or ~/.local/share/applications which is used for launching Evolution.")
	databasesPaths         = make([]string, 0)

	dbusSession *dbus.Conn
)

func main() {
	// CTRL+C handler.
	signalHandler := make(chan os.Signal, 1)
	shutdownDone := make(chan bool, 1)

	signal.Notify(signalHandler, os.Interrupt, syscall.SIGTERM)

	slog.Info("Starting Evolved...")

	flag.Parse()
	if *cliFlagIsDebug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

		slog.Debug("Debug output enabled!")
	}

	dbusConn, err := dbus.ConnectSessionBus()
	if err != nil {
		slog.Error("Failed to connect to dbus session bus!", "error", err)

		os.Exit(3)
	}

	dbusSession = dbusConn

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Failed to start filesystem changes watcher!", "error", err)
		os.Exit(1)
	}

	if err := getEvolutionMailDatabasesPaths(); err != nil {
		slog.Error("Failed to get Evolution mail databases paths!", "error", err)
		os.Exit(2)
	}

	for _, databasePath := range databasesPaths {
		slog.Info("Starting listening for filesystem changes in Evolution directory...", "path", databasePath)

		_ = watcher.Add(databasePath)
	}

	go watchFSNotifications(watcher)

	unreadCount, err := getEvolutionUnreadMailsCount()
	if err != nil {
		slog.Error("Failed to get unread counts!", "error", err)

		os.Exit(4)
	}

	emitDBusSignal(unreadCount)

	slog.Info("Evolved started.")

	go func() {
		<-signalHandler

		slog.Info("Shutting down Evolved...")

		if err := watcher.Close(); err != nil {
			slog.Error("Failed to stop filesystem watcher!", "error", err)
		}

		emitDBusSignal(0)

		shutdownDone <- true
	}()

	<-shutdownDone

	slog.Info("Evolved stopped.")

	os.Exit(0)
}

func emitDBusSignal(unreadCount uint) {
	params := make(map[string]dbus.Variant)
	params["count"] = dbus.MakeVariant(unreadCount)
	params["count-visible"] = dbus.MakeVariant(unreadCount > 0)

	if err := dbusSession.Emit(
		"/",
		"com.canonical.Unity.LauncherEntry.Update",
		"application://"+*cliFlagDesktopFileName,
		params,
	); err != nil {
		slog.Error("Failed to emit badge data via dbus!", "error", err)
	}
}

func getEvolutionMailDatabasesPaths() error {
	userData, err := user.Current()
	if err != nil {
		return fmt.Errorf("getEvolutionMailDatabasesPaths: get current user: %w", err)
	}

	if err := filepath.Walk(path.Join(userData.HomeDir, ".cache", "evolution", "mail"), func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(info.Name(), "folders.db") {
			databasesPaths = append(databasesPaths, path)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("getEvolutionMailDatabasesPaths: get databases paths: %w", err)
	}

	return nil
}

func getEvolutionUnreadMailsCount() (uint, error) {
	unreadCount := uint(0)

	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)

	for _, dbFile := range databasesPaths {
		db, err := sqlx.Connect("sqlite3", dbFile)
		if err != nil {
			slog.Error("Failed to open Evolution database file!", "file", dbFile, "error", err)

			continue
		}

		counts := make([]uint, 0)

		if err := db.SelectContext(ctx, &counts, "SELECT unread_count FROM folders"); err != nil {
			slog.Error("Failed to get unread counts!", "file", dbFile, "error", err)

			continue
		}

		for _, count := range counts {
			unreadCount += count
		}

		if err := db.Close(); err != nil {
			slog.Error("Failed to close database file for reading! Expect unexpected!", "file", dbFile, "error", err)

			continue
		}
	}

	return unreadCount, nil
}

func watchFSNotifications(watcher *fsnotify.Watcher) {
	slog.Debug("Starting filesystem watcher goroutine...")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			slog.Debug("Got filesystem event", "event", event.Name, "op", event.Op.String())

			unreadCount, err := getEvolutionUnreadMailsCount()
			if err != nil {
				slog.Error("Failed to get unread count in watcher goroutine!", "error", err)

				continue
			}

			slog.Info("Got unread count", "count", unreadCount)

			emitDBusSignal(unreadCount)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}

			slog.Error("Got error from filesystem watcher!", "error", err)
		}
	}
}
