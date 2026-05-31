package telegram

import (
	"log/slog"
	"time"

	"pkm-daemon/internal/ai"
	"pkm-daemon/internal/fsm"
	"pkm-daemon/internal/storage"

	telebot "gopkg.in/telebot.v3"
)

type Config struct {
	Token        string
	AdminID      int64
	GuestID      int64
	ObsidianPath string
}

type Bot struct {
	bot *telebot.Bot
	cfg Config
	ai  *ai.Client
	db  *storage.Storage
	fsm *fsm.Manager
	log *slog.Logger
}

func NewBot(cfg Config, aiClient *ai.Client, db *storage.Storage, logger *slog.Logger) (*Bot, error) {
	pref := telebot.Settings{
		Token:  cfg.Token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}

	telegramBot := &Bot{
		bot: b,
		cfg: cfg,
		ai:  aiClient,
		db:  db,
		fsm: fsm.NewManager(db),
		log: logger,
	}

	b.Use(telegramBot.authMiddleware())

	b.Handle("/tasks", telegramBot.handleTasksCommand)
	b.Handle(telebot.OnText, telegramBot.handleText)

	b.Handle("\ft_done", telegramBot.handleTaskDoneCallback)
	b.Handle("\ft_fail", telegramBot.handleTaskFailCallback)

	return telegramBot, nil
}

func (b *Bot) Start() {
	b.log.Info("Telegram bot is starting...")
	b.bot.Start()
}

func (b *Bot) Stop() {
	b.log.Info("Stopping Telegram bot...")
	b.bot.Stop()
}

// Bot возвращает нативный инстанс telebot для работы извне (например, из Планировщика)
func (b *Bot) Bot() *telebot.Bot {
	return b.bot
}