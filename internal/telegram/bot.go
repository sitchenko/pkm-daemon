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
	log *slog.Logger
	fsm *fsm.Manager
}

func NewBot(cfg Config, aiClient *ai.Client, db *storage.Storage, log *slog.Logger) (*Bot, error) {
	pref := telebot.Settings{
		Token:  cfg.Token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}

	fsmManager := fsm.NewManager(db)

	bot := &Bot{
		bot: b,
		cfg: cfg,
		ai:  aiClient,
		db:  db,
		log: log,
		fsm: fsmManager,
	}

	bot.bot.Use(bot.authMiddleware())
	bot.setupHandlers()

	// Синхронизация Этапа 15 (если уже подключено, не теряем)
	RegisterSyncHandlers(bot.bot, db, cfg.ObsidianPath, log)

	return bot, nil
}

func (b *Bot) Bot() *telebot.Bot {
	return b.bot
}

func (b *Bot) Start() {
	b.log.Info("Starting Telegram bot...")
	b.bot.Start()
}

func (b *Bot) Stop() {
	b.log.Info("Stopping Telegram bot...")
	b.bot.Stop()
}

func (b *Bot) setupHandlers() {
	// Базовый текстовый обработчик
	b.bot.Handle(telebot.OnText, b.handleText)

	// Обработчики медиафайлов (Этап 16)
	b.bot.Handle(telebot.OnPhoto, b.handleMedia)
	b.bot.Handle(telebot.OnVoice, b.handleMedia)
	b.bot.Handle(telebot.OnVideo, b.handleMedia)
	b.bot.Handle(telebot.OnDocument, b.handleMedia)
}
