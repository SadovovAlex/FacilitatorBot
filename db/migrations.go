package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

func RunMigrations(db *sql.DB) error {
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
			        ai_user_info TEXT,
			        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			    );

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
			name: "seed_chats_data",
			sql: `
               INSERT OR IGNORE INTO chats (id, title, type, username, created_at) VALUES
               (-1002748220550, 'AdminBot3', 'supergroup', '', '2025-07-23 19:53:25'),
               (-1002631108476, 'AdminBot', 'supergroup', '', '2025-05-22 18:34:57'),
               (-1002478281670, 'Атипичный чат', 'supergroup', 'fans_kadabrus', '2025-05-13 11:52:03'),
               (-1002407860030, 'AdminBot2', 'supergroup', '', '2025-05-21 10:18:21'),
               (-226919585, 'АвтоМотоБаггиРетроЯхтингВертолётингИнфоПечкинг', 'group', '', '2025-07-18 16:44:15');
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

				CREATE INDEX IF NOT EXISTS idx_context_chat_user ON chat_context(chat_id, user_id);
                CREATE INDEX IF NOT EXISTS idx_context_timestamp ON chat_context(timestamp);
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

				CREATE INDEX IF NOT EXISTS idx_ai_billing_user ON ai_billing(user_id);
                CREATE INDEX IF NOT EXISTS idx_ai_billing_timestamp ON ai_billing(timestamp);
            `,
		},
		{
			name: "add_users_role_table",
			sql: `
                CREATE TABLE IF NOT EXISTS users_role (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    user_id INTEGER NOT NULL,
                    chat_id INTEGER NOT NULL,
                    role TEXT NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    FOREIGN KEY (user_id) REFERENCES users(id),
                    FOREIGN KEY (chat_id) REFERENCES chats(id),
                    UNIQUE(user_id, chat_id)
                );

				CREATE INDEX IF NOT EXISTS idx_users_role_user ON users_role(user_id);
                CREATE INDEX IF NOT EXISTS idx_users_role_chat ON users_role(chat_id);
            `,
		},
		// спасибо
		{
			name: "add_thanks_module",
			sql: `
                CREATE TABLE IF NOT EXISTS mod_thanks (
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

				CREATE INDEX IF NOT EXISTS idx_thanks_from_user ON mod_thanks(from_user_id);
                CREATE INDEX IF NOT EXISTS idx_thanks_to_user ON mod_thanks(to_user_id);
                CREATE INDEX IF NOT EXISTS idx_thanks_chat ON mod_thanks(chat_id);
            `,
		},
		// антиспам
		{
			name: "add_thanks_module",
			sql: `   
    		CREATE TABLE IF NOT EXISTS mod_spam_incidents (
			        id INTEGER PRIMARY KEY AUTOINCREMENT,
			        chat_id INTEGER,
			        user_id INTEGER,
			        message_text TEXT,
					reason TEXT,
			        created_at TIMESTAMP,
			        FOREIGN KEY(chat_id) REFERENCES chats(id),
			        FOREIGN KEY(user_id) REFERENCES users(id)
			    );

				CREATE INDEX IF NOT EXISTS idx_incidents_chat_user ON incidents(chat_id, user_id);
        		CREATE INDEX IF NOT EXISTS idx_incidents_timestamp ON incidents(created_at);

            `,
		},
		// капча
		{
			name: "add_captchas_module",
			sql: `
        CREATE TABLE IF NOT EXISTS captchas (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            chat_id INTEGER NOT NULL,
            user_id INTEGER NOT NULL,
            question TEXT NOT NULL,
            answer INTEGER NOT NULL,
            sent_at TIMESTAMP NOT NULL,
            answered_at TIMESTAMP NULL,
            is_correct BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (user_id) REFERENCES users(id)
        );

        CREATE INDEX IF NOT EXISTS idx_captchas_chat_user ON captchas(chat_id, user_id);
        CREATE INDEX IF NOT EXISTS idx_captchas_sent_at ON captchas(sent_at);
        CREATE INDEX IF NOT EXISTS idx_captchas_active ON captchas(chat_id, user_id, answered_at);
    `,
		},
	}

	// Создаем таблицу для отслеживания выполненных миграций
	if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS db_migrations (
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
		err := db.QueryRow("SELECT COUNT(*) FROM db_migrations WHERE name = ?", migration.name).Scan(&count)
		if err != nil {
			return fmt.Errorf("ошибка проверки миграции %s: %v", migration.name, err)
		}

		if count == 0 {
			// Выполняем миграцию
			if _, err := db.Exec(migration.sql); err != nil {
				// Игнорируем ошибки "duplicate column" и "index already exists"
				if !strings.Contains(err.Error(), "duplicate column") &&
					!strings.Contains(err.Error(), "already exists") &&
					!strings.Contains(err.Error(), "duplicate index") {
					return fmt.Errorf("ошибка выполнения миграции %s: %v", migration.name, err)
				}
			}

			// Помечаем миграцию как выполненную
			if _, err := db.Exec("INSERT INTO db_migrations (name) VALUES (?)", migration.name); err != nil {
				return fmt.Errorf("ошибка записи миграции %s: %v", migration.name, err)
			}

			log.Printf("Применена миграция: %s", migration.name)
		}
	}

	return nil
}
