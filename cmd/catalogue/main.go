package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/locale"
	"ai-venture/internal/terminal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	language, err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		terminal.NewPrinter(os.Stderr, terminal.ColorAuto).Println(terminal.StyleError, language.Text("Error:")+" "+language.Error(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output, diagnostics io.Writer) (locale.Language, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-help" || args[0] == "--help" {
		_, err := fmt.Fprintln(output, "Usage: catalogue import|list|search|show|assets [options]\n  -db catalogue.sqlite -lang en|fr\n  import -source folder-or-manifest.json [-replace]\n  list [-kind location|character|monster|item|situation] [-limit 20]\n  search -query words [-kind ...] [-limit 20]\n  show -id entry_id\n  assets\nOutput is JSON. Import updates matching IDs; -replace explicitly replaces the whole catalogue.")
		return locale.English, err
	}
	command := args[0]
	flags := flag.NewFlagSet("catalogue "+command, flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	db := flags.String("db", "catalogue.sqlite", "Permanent catalogue database")
	langFlag := flags.String("lang", "en", "Language: en or fr")
	source := flags.String("source", "", "Manifest file or directory to import")
	replace := flags.Bool("replace", false, "Replace the complete catalogue (otherwise update/insert)")
	kind := flags.String("kind", "", "Entry kind")
	query := flags.String("query", "", "Literal full-text search terms")
	id := flags.String("id", "", "Entry identifier")
	limit := flags.Int("limit", 20, "Maximum search/list results (1-100)")
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return locale.English, nil
		}
		return locale.English, err
	}
	language, err := locale.Parse(*langFlag)
	if err != nil {
		return language, err
	}
	if flags.NArg() != 0 {
		return language, locale.Errorf("unexpected arguments; use -help")
	}
	if command != "import" && command != "list" && command != "search" && command != "show" && command != "assets" {
		return language, locale.Errorf("unknown catalogue command: %s", command)
	}
	var batch catalogue.Batch
	if command == "import" {
		if *source == "" {
			return language, locale.Errorf("catalogue import requires -source")
		}
		batch, err = catalogue.Load(ctx, *source)
		if err != nil {
			return language, err
		}
	} else if *source != "" || *replace {
		return language, locale.Errorf("catalogue import options require the import command")
	}
	if command == "search" && *query == "" {
		return language, locale.Errorf("catalogue search requires -query")
	}
	if command == "show" && *id == "" {
		return language, locale.Errorf("catalogue show requires -id")
	}
	store, err := catalogue.Open(ctx, *db, command == "import")
	if err != nil {
		return language, err
	}
	defer store.Close()
	var result any
	switch command {
	case "import":
		result, err = store.Import(ctx, batch, *replace)
	case "show":
		result, err = store.Entry(ctx, *id, string(language))
	case "assets":
		result, err = store.Assets(ctx)
	case "list", "search":
		result, err = store.Search(ctx, catalogue.Query{Language: string(language), Kind: *kind, Text: *query, Limit: *limit})
	}
	if err != nil {
		return language, err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return language, encoder.Encode(result)
}
