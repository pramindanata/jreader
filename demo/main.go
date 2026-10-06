package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pramindanata/fread"
)

const interval = 3 * time.Second

var files = []string{
	"demo/first.json",
	"demo/second.json",
}

type payload struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fileReader, err := fread.New(logger)

	if err != nil {
		logger.Error("failed to create reader", "error", err)
		os.Exit(1)
	}

	defer fileReader.Close()

	fileReader.Start()
	logger.Info("watching files", "files", files, "interval", interval.String())
	logger.Info("edit any json file below to see the cache invalidate; press Ctrl+C to stop")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			printContents(fileReader)
		case sig := <-signals:
			logger.Info("stopping", "signal", sig.String())

			return
		}
	}
}

func printContents(fileReader *fread.Read) {
	fmt.Println("----------------------------------------")

	for _, path := range files {
		var data payload

		if err := fileReader.Read(path, &data); err != nil {
			fmt.Printf("%s: %v\n", path, err)
			continue
		}

		fmt.Printf("%s: %+v\n", path, data)
	}

	fmt.Printf("printed at %s\n", time.Now().Format(time.TimeOnly))
}
