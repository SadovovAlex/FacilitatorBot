package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) initDB() error {
	// Список миграций в порядке их применения
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "initial_schema",
			sql: `
                CREATE TABLE IF NOT EXISTS chats (
                    id INTEGER PRIMARY KEY,
                    title TEXT,
                    type TEXT,
                    username TEXT,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                );
                
                CREATE TABLE IF NOT EXISTS users (
                    id INTEGER PRIMARY KEY,
                    username TEXT,
                    first_name TEXT,
                    last_name TEXT,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                );
            `,
		},
		{
			name: "add_messages_table",
			sql: `
                CREATE TABLE IF NOT EXISTS messages (
                    id INTEGER PRIMARY KEY,
                    chat_id INTEGER,
                    user_id INTEGER,
                    text TEXT,
                    timestamp INTEGER,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY(chat_id) REFERENCES chats(id),
                    FOREIGN KEY(user_id) REFERENCES users(id)
                );
            `,
		},
		{
			name: "add_thanks_table",
			sql: `
                CREATE TABLE IF NOT EXISTS thanks (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    chat_id INTEGER NOT NULL,
                    from_user_id INTEGER NOT NULL,
                    to_user_id INTEGER NOT NULL,
                    text TEXT NOT NULL,
                    timestamp INTEGER NOT NULL,
                    message_id INTEGER NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (from_user_id) REFERENCES users(id),
                    FOREIGN KEY (to_user_id) REFERENCES users(id)
                );
            `,
		},
		{
			name: "add_chat_context_table",
			sql: `
                CREATE TABLE IF NOT EXISTS chat_context (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    chat_id INTEGER NOT NULL,
                    user_id INTEGER NOT NULL,
                    role TEXT NOT NULL,
                    content TEXT NOT NULL,
                    timestamp INTEGER NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (chat_id) REFERENCES chats(id),
                    FOREIGN KEY (user_id) REFERENCES users(id)
                );
            `,
		},
		{
			name: "add_ai_billing_table",
			sql: `
                CREATE TABLE IF NOT EXISTS ai_billing (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    user_id INTEGER NOT NULL,
                    chat_id INTEGER NOT NULL,
                    timestamp INTEGER NOT NULL,
                    model TEXT NOT NULL,
                    prompt_tokens INTEGER NOT NULL,
                    completion_tokens INTEGER NOT NULL,
                    total_tokens INTEGER NOT NULL,
                    cost REAL NOT NULL,
                    FOREIGN KEY (user_id) REFERENCES users(id),
                    FOREIGN KEY (chat_id) REFERENCES chats(id)
                );
            `,
		},
		{
			name: "add_users_role_table",
			sql: `
                CREATE TABLE IF NOT EXISTS users_role (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    user_id INTEGER NOT NULL,
                    channel_id INTEGER NOT NULL,
                    role TEXT NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (user_id) REFERENCES users(id),
                    FOREIGN KEY (channel_id) REFERENCES chats(id),
                    UNIQUE(user_id, channel_id)
                );
            `,
		},
		{
			name: "add_ai_user_info_column",
			sql:  `ALTER TABLE users ADD COLUMN ai_user_info TEXT;`,
		},
		{
			name: "fix_thanks_foreign_keys",
			sql: `
                PRAGMA foreign_keys=off;
                
                BEGIN TRANSACTION;
                
                CREATE TABLE IF NOT EXISTS thanks_new (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    chat_id INTEGER NOT NULL,
                    from_user_id INTEGER NOT NULL,
                    to_user_id INTEGER NOT NULL,
                    text TEXT NOT NULL,
                    timestamp INTEGER NOT NULL,
                    message_id INTEGER NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (from_user_id) REFERENCES users(id),
                    FOREIGN KEY (to_user_id) REFERENCES users(id)
                );
                
                INSERT INTO thanks_new SELECT * FROM thanks;
                DROP TABLE thanks;
                ALTER TABLE thanks_new RENAME TO thanks;
                
                COMMIT;
                
                PRAGMA foreign_keys=on;
            `,
		},
		{
			name: "create_initial_indexes",
			sql: `
                CREATE INDEX IF NOT EXISTS idx_thanks_from_user ON thanks(from_user_id);
                CREATE INDEX IF NOT EXISTS idx_thanks_to_user ON thanks(to_user_id);
                CREATE INDEX IF NOT EXISTS idx_thanks_chat ON thanks(chat_id);
            `,
		},
		{
			name: "create_context_indexes",
			sql: `
                CREATE INDEX IF NOT EXISTS idx_context_chat_user ON chat_context(chat_id, user_id);
                CREATE INDEX IF NOT EXISTS idx_context_timestamp ON chat_context(timestamp);
            `,
		},
		{
			name: "create_ai_billing_indexes",
			sql: `
                CREATE INDEX IF NOT EXISTS idx_ai_billing_user ON ai_billing(user_id);
                CREATE INDEX IF NOT EXISTS idx_ai_billing_timestamp ON ai_billing(timestamp);
            `,
		},
		{
			name: "create_users_role_indexes",
			sql: `
                CREATE INDEX IF NOT EXISTS idx_users_role_user ON users_role(user_id);
                CREATE INDEX IF NOT EXISTS idx_users_role_channel ON users_role(channel_id);
            `,
		},
	}

	// Создаем таблицу для отслеживания выполненных миграций
	if _, err := b.db.Exec(`
        CREATE TABLE IF NOT EXISTS migrations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT UNIQUE NOT NULL,
            executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
    `); err != nil {
		return fmt.Errorf("ошибка создания таблицы миграций: %v", err)
	}

	// Применяем миграции
	for _, migration := range migrations {
		// Проверяем, была ли уже выполнена эта миграция
		var count int
		err := b.db.QueryRow("SELECT COUNT(*) FROM migrations WHERE name = ?", migration.name).Scan(&count)
		if err != nil {
			return fmt.Errorf("ошибка проверки миграции %s: %v", migration.name, err)
		}

		if count == 0 {
			// Выполняем миграцию
			if _, err := b.db.Exec(migration.sql); err != nil {
				// Игнорируем ошибки "duplicate column" и "index already exists"
				if !strings.Contains(err.Error(), "duplicate column") &&
					!strings.Contains(err.Error(), "already exists") &&
					!strings.Contains(err.Error(), "duplicate index") {
					return fmt.Errorf("ошибка выполнения миграции %s: %v", migration.name, err)
				}
			}

			// Помечаем миграцию как выполненную
			if _, err := b.db.Exec("INSERT INTO migrations (name) VALUES (?)", migration.name); err != nil {
				return fmt.Errorf("ошибка записи миграции %s: %v", migration.name, err)
			}

			log.Printf("Применена миграция: %s", migration.name)
		}
	}

	return nil
}

// saveChat сохраняет информацию о чате в БД
func (b *Bot) saveChat(chat *tgbotapi.Chat) error {
	if chat == nil {
		return nil
	}

	_, err := b.db.Exec(`
		INSERT OR IGNORE INTO chats (id, title, type, username) 
		VALUES (?, ?, ?, ?)`,
		chat.ID, chat.Title, chat.Type, chat.UserName)

	return err
}

// saveUser сохраняет информацию о пользователе в БД, 136817688  это сообщения от имени канала
func (b *Bot) saveUser(user *tgbotapi.User) error {
	if user == nil {
		return nil
	}

	firstName := user.FirstName
	if user.ID == 136817688 {
		firstName = "Админ-Канала"
	}

	result, err := b.db.Exec(`
		INSERT OR IGNORE INTO users (id, username, first_name, last_name) 
		VALUES (?, ?, ?, ?)`,
		user.ID, user.UserName, firstName, user.LastName)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected > 0 {
		fmt.Printf("Saved user: ID=%d, Username=%s, FirstName=%s, LastName=%s", user.ID, user.UserName, user.FirstName, user.LastName)
	}

	return nil
}

// saveMessage сохраняет сообщение в БД
func (b *Bot) saveMessage(chatID, userID int64, text string, timestamp int64) error {
	_, err := b.db.Exec(`
		INSERT INTO messages (chat_id, user_id, text, timestamp) 
		VALUES (?, ?, ?, ?)`,
		chatID, userID, text, timestamp)

	return err
}

// saveThanks сохраняет благодарность с информацией о получателе
func (b *Bot) saveThanks(chatID, fromUserID, toUserID int64, text string, timestamp int64, messageID int) error {
	_, err := b.db.Exec(`
        INSERT INTO thanks (chat_id, from_user_id, to_user_id, text, timestamp, message_id) 
        VALUES (?, ?, ?, ?, ?, ?)`,
		chatID, fromUserID, toUserID, text, timestamp, messageID)

	return err
}

// getUserByUsername получает пользователя по username из БД
func (b *Bot) getUserByUsername(username string) (*tgbotapi.User, error) {
	// Реализация зависит от вашей структуры БД
	// Примерная реализация:
	var user tgbotapi.User
	err := b.db.QueryRow(`
        SELECT user_id, first_name, last_name, username 
        FROM users 
        WHERE username = ?`, username).Scan(
		&user.ID, &user.FirstName, &user.LastName, &user.UserName)

	if err != nil {
		return nil, err
	}
	return &user, nil
}

// getRecentMessages получает сообщения за последние [limit] часов
func (b *Bot) getRecentMessages(chatID int64, limit int) ([]DBMessage, error) {
	hoursAgo := time.Now().Add(CHECK_HOURS * time.Hour).Unix()

	// Если лимит не задан, устанавливаем его в 0, чтобы получить все сообщения
	if limit == 0 {
		limit = -1
	}

	query := `
		SELECT m.id, m.chat_id, m.user_id, u.username, u.first_name, u.last_name, m.text, m.timestamp, 
		       c.title as chat_title
		FROM messages m
		LEFT JOIN users u ON m.user_id = u.id
		LEFT JOIN chats c ON m.chat_id = c.id
		WHERE m.timestamp >= ? 
		AND m.chat_id = ?
		ORDER BY m.timestamp desc
		LIMIT ?
	`

	rows, err := b.db.Query(query, hoursAgo, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса сообщений: %v", err)
	}
	defer rows.Close()

	var messages []DBMessage
	for rows.Next() {
		var msg DBMessage
		err := rows.Scan(
			&msg.ID,
			&msg.ChatID,
			&msg.UserID,
			&msg.Username,
			&msg.UserFirstName,
			&msg.UserLastName,
			&msg.Text,
			&msg.Timestamp,
			&msg.ChatTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения сообщения: %v", err)
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка обработки результатов: %v", err)
	}

	//fmt.Printf("mmmm: %--v", messages)

	return messages, nil
}

// saveContext сохраняет контекст сообщения в БД
func (b *Bot) saveContext(chatID, userID int64, role, content string, timestamp int64) error {
	_, err := b.db.Exec(`
		INSERT INTO chat_context (chat_id, user_id, role, content, timestamp) 
		VALUES (?, ?, ?, ?, ?)`,
		chatID, userID, role, content, timestamp)
	return err
}

// getConversationContext получает контекст общения для указанного чата и пользователя
// limitMessages - максимальное количество сообщений для возврата (0 - без ограничения)
// timeLimitHours - максимальный возраст сообщений в часах (0 - без ограничения)
func (b *Bot) getConversationContext(chatID, userID int64, limitMessages int, timeLimitHours int) ([]ContextMessage, error) {
	var query string
	var args []interface{}

	query = `
		SELECT role, content, timestamp 
		FROM chat_context 
		WHERE chat_id = ? AND user_id = ?
	`

	args = append(args, chatID, userID)

	if timeLimitHours > 0 {
		timeLimit := time.Now().Add(-time.Duration(timeLimitHours) * time.Hour).Unix()
		query += " AND timestamp >= ?"
		args = append(args, timeLimit)
	}

	query += " ORDER BY timestamp DESC"

	if limitMessages > 0 {
		query += " LIMIT ?"
		args = append(args, limitMessages)
	}

	rows, err := b.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса контекста: %v", err)
	}
	defer rows.Close()

	var context []ContextMessage
	for rows.Next() {
		var msg ContextMessage
		err := rows.Scan(&msg.Role, &msg.Content, &msg.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения контекста: %v", err)
		}
		context = append(context, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка обработки результатов: %v", err)
	}

	// Переворачиваем порядок, чтобы хронология была правильной
	for i, j := 0, len(context)-1; i < j; i, j = i+1, j-1 {
		context[i], context[j] = context[j], context[i]
	}

	return context, nil
}

// DeleteUserContext удаляет контекст общения для указанного пользователя в чате
func (b *Bot) DeleteUserContext(chatID, userID int64) error {
	if b.db == nil {
		return fmt.Errorf("база данных не инициализирована")
	}

	_, err := b.db.Exec(`
        DELETE FROM chat_context 
        WHERE chat_id = ? AND user_id = ?`,
		chatID, userID)

	if err != nil {
		return fmt.Errorf("ошибка удаления контекста: %v", err)
	}

	log.Printf("Контекст удален для user_id %d в chat_id %d", userID, chatID)
	return nil
}

// SaveBillingRecord сохраняет информацию об использовании токенов
func (b *Bot) SaveBillingRecord(record BillingRecord) error {
	_, err := b.db.Exec(`
        INSERT INTO ai_billing 
        (user_id, chat_id, timestamp, model, prompt_tokens, completion_tokens, total_tokens, cost)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.UserID,
		record.ChatID,
		record.Timestamp,
		record.Model,
		record.PromptTokens,
		record.CompletionTokens,
		record.TotalTokens,
		record.Cost)

	if err != nil {
		return fmt.Errorf("ошибка сохранения записи биллинга: %v", err)
	}
	return nil
}

// GetChatTokenUsage возвращает статистику использования токенов в чате
func (b *Bot) GetChatTokenUsage(chatID int64, days int) (BillingRecord, error) {
	var result BillingRecord
	var threshold int64

	if days > 0 {
		threshold = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}

	query := `
        SELECT 
            SUM(prompt_tokens) as prompt_tokens,
            SUM(completion_tokens) as completion_tokens,
            SUM(total_tokens) as total_tokens,
            SUM(cost) as cost
        FROM ai_billing
        WHERE chat_id = ?`

	args := []interface{}{chatID}

	if days > 0 {
		query += " AND timestamp >= ?"
		args = append(args, threshold)
	}

	err := b.db.QueryRow(query, args...).Scan(
		&result.PromptTokens,
		&result.CompletionTokens,
		&result.TotalTokens,
		&result.Cost)

	if err != nil {
		return BillingRecord{}, fmt.Errorf("ошибка получения статистики токенов: %v", err)
	}

	result.ChatID = chatID
	return result, nil
}

func (b *Bot) GetTopUsersByTokenUsage(limit int, days int) ([]struct {
	UserID      int64
	TotalTokens int
	Cost        float64
}, error) {
	var threshold int64
	if days > 0 {
		threshold = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}

	query := `
        SELECT 
            user_id,
            SUM(total_tokens) as total_tokens,
            SUM(cost) as cost
        FROM ai_billing`

	args := []interface{}{}

	if days > 0 {
		query += " WHERE timestamp >= ?"
		args = append(args, threshold)
	}

	query += " GROUP BY user_id ORDER BY total_tokens DESC LIMIT ?"
	args = append(args, limit)

	rows, err := b.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса топ пользователей: %v", err)
	}
	defer rows.Close()

	var result []struct {
		UserID      int64
		TotalTokens int
		Cost        float64
	}

	for rows.Next() {
		var item struct {
			UserID      int64
			TotalTokens int
			Cost        float64
		}
		if err := rows.Scan(&item.UserID, &item.TotalTokens, &item.Cost); err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, nil
}

// cleanupOldContext удаляет старый контекст общения
func (b *Bot) cleanupOldContext() {
	for {
		time.Sleep(12 * time.Hour) // Проверяем каждые 12 часов
		// Удаляем контекст старше 7 дней (или другого значения из конфига)
		threshold := time.Now().Add(-time.Duration(b.config.ContextRetentionDays) * 24 * time.Hour).Unix()
		_, err := b.db.Exec("DELETE FROM chat_context WHERE timestamp < ?", threshold)
		if err != nil {
			log.Printf("Ошибка очистки старого контекста: %v", err)
		}
	}
}

// DeleteOldMessages удаляет сообщения старше указанного количества дней
func (b *Bot) DeleteOldMessages() error {
	for {
		time.Sleep(12 * time.Hour) // Проверяем каждые 12 часов
		threshold := time.Now().Add(-time.Duration(b.config.HistoryDays) * 24 * time.Hour).Unix()
		_, err := b.db.Exec(`
			DELETE FROM messages 
			WHERE timestamp < ?`,
			threshold)

		if err != nil {
			log.Printf("ошибка удаления старых сообщений: %v", err)
		}

		log.Printf("Удалены сообщения старше %d дней", b.config.HistoryDays)
	}
}
