package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/frederic/tgtldr/app/internal/api"
	"github.com/frederic/tgtldr/app/internal/bot"
	"github.com/frederic/tgtldr/app/internal/botquery"
	"github.com/frederic/tgtldr/app/internal/clock"
	"github.com/frederic/tgtldr/app/internal/config"
	"github.com/frederic/tgtldr/app/internal/knowledge"
	"github.com/frederic/tgtldr/app/internal/llmwiki"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/scheduler"
	"github.com/frederic/tgtldr/app/internal/store"
	"github.com/frederic/tgtldr/app/internal/summary"
	telegramsvc "github.com/frederic/tgtldr/app/internal/telegram"
	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := store.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := store.RunMigrations(ctx, st); err != nil {
		return err
	}

	sysClock := clock.System{}
	botService := bot.New()
	summaryService := summary.NewService(st, sysClock, cfg.OpenAITimeout)
	summaryAggregator := summary.NewAggregator(st, sysClock, cfg.OpenAITimeout)
	knowledgeService := knowledge.NewService(st, sysClock, cfg.OpenAITimeout)
	llmWikiService := llmwiki.NewService(st, cfg.LLMWikiDir, cfg.OpenAITimeout)
	if err := llmWikiService.EnsureWorkspace(); err != nil {
		return fmt.Errorf("ensure llm wiki workspace: %w", err)
	}
	if _, _, err := knowledgeService.EnsureDefaultGeneralSpace(ctx); err != nil {
		return fmt.Errorf("ensure default knowledge space: %w", err)
	}
	botQueryService := botquery.NewService(st, botService, knowledgeService)
	if settings, err := st.Settings.Get(ctx); err == nil {
		if err := botquery.SyncBotCommands(ctx, botService, settings); err != nil {
			fmt.Fprintf(os.Stderr, "sync bot commands: %v\n", err)
		}
	}
	telegramService := telegramsvc.NewService(ctx, st, sysClock)
	schedulerService := scheduler.NewService(st, sysClock, summaryService, botService, knowledgeService, summaryAggregator, llmWikiService)
	telegramService.SetHistoryBackfillCompletionHook(func(chat model.Chat, fromDate, toDate string) {
		_ = schedulerService.RepairEmptySummariesInRange(context.Background(), chat, fromDate, toDate)
	})
	router := api.New(
		st,
		telegramService,
		schedulerService,
		botService,
		knowledgeService,
		llmWikiService,
		cfg.WebOrigin,
		cfg.RequestTimout,
	)

	if auth, err := telegramService.BootstrapAuth(ctx); err == nil && auth != nil && auth.Status == "authorized" {
		telegramService.EnsureListener()
	}

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router.Handler(),
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := schedulerService.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		if err := botQueryService.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		<-groupCtx.Done()
		telegramService.StopListener()
		return server.Shutdown(context.Background())
	})

	return group.Wait()
}
