package recovery

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"
)

// RunWithRecovery выполняет переданную функцию и перехватывает паники.
func RunWithRecovery(logger *slog.Logger, crashLogPath string, fn func()) {
	defer func() {
		if err := recover(); err != nil {
			stackTrace := string(debug.Stack())
			
			// Записываем критическую ошибку в основной логгер
			logger.Error("PANIC RECOVERED", 
				slog.Any("error", err),
			)

			// Изолированно пишем трассировку стека в crashlogs.log
			writeCrashLog(crashLogPath, err, stackTrace)
		}
	}()
	fn()
}

// Go запускает безопасную горутины с автоматическим перехватом паники.
func Go(logger *slog.Logger, crashLogPath string, fn func()) {
	go RunWithRecovery(logger, crashLogPath, fn)
}

func writeCrashLog(path string, err any, stack string) {
	f, fileErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to open crashlog file: %v\n", fileErr)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format(time.RFC3339)
	crashMsg := fmt.Sprintf("========== CRASH LOG [%s] ==========\nError: %v\nStack Trace:\n%s\n==================================================\n\n", timestamp, err, stack)
	
	_, _ = f.WriteString(crashMsg)
}