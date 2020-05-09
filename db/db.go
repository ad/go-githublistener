package db

import (
	s "database/sql"
	"fmt"
	"log"
	"reflect"
	"time"

	sql "github.com/lazada/sqle"
	_ "github.com/mattn/go-sqlite3" // ...
)

// GithubUser ...
type GithubUser struct {
	ID        int
	Name      string
	UserName  string
	Token     string
	CreatedAt time.Time
}

// TelegramMessage ...
type TelegramMessage struct {
	ID       int
	UserID   int
	UserName string
	Message  string
	Date     time.Time
}

// InitDB ...
func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "db/githublistener.db")
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	err = ExecSQL(db, `CREATE TABLE IF NOT EXISTS telegram_messages (
		"id" INTEGER PRIMARY KEY AUTOINCREMENT,
		"user_id" INTEGER NOT NULL,
		"user_name" VARCHAR(32) DEFAULT "",
		"message" TEXT DEFAULT "",
		"created_at" timestamp DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		fmt.Println(err)
	}

	err = ExecSQL(db, `CREATE TABLE IF NOT EXISTS "github_users" (
		"id" INTEGER PRIMARY KEY AUTOINCREMENT,
		"name" INTEGER NOT NULL,
		"user_name" text NOT NULL,
		"token" timestamp,
		"created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT "github_users_user_name" UNIQUE ("user_name") ON CONFLICT IGNORE
	  );`)
	if err != nil {
		fmt.Println(err)
	}

	// CREATE INDEX IF NOT EXISTS "messages_user_id"
	// ON "messages" (
	//   "user_id" ASC
	// );

	var returnModel TelegramMessage

	result, err := QuerySQLList(db, `select * from telegram_messages order by id desc limit 0, 5`, returnModel)
	if err != nil {
		log.Println(err)
	}
	for _, item := range result {
		if returnModel, ok := item.Interface().(*TelegramMessage); ok {
			// FIXME: some problem with time.Time :(
			log.Printf("%s: %s [%d] %s", returnModel.Date, returnModel.UserName, returnModel.UserID, returnModel.Message)
		}
	}

	return db, nil
}

// ExecSQL ...
func ExecSQL(db *sql.DB, sql string) error {
	_, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), sql)
	}

	return nil
}

// QuerySQLObject ...
func QuerySQLObject(db *sql.DB, sql string, returnModel interface{}) (reflect.Value, error) {
	t := reflect.TypeOf(returnModel)
	u := reflect.New(t)

	err := db.QueryRow(sql).Scan(u.Interface())
	switch {
	case err == s.ErrNoRows:
		return u, nil
	case err != nil:
		return u, fmt.Errorf("%s: %s", err.Error(), sql)
	}

	return u, nil
}

// QuerySQLList ...
func QuerySQLList(db *sql.DB, sql string, returnModel interface{}) ([]reflect.Value, error) {
	var result []reflect.Value

	rows, err := db.Query(sql)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err.Error(), sql)
	}

	t := reflect.TypeOf(returnModel)

	for rows.Next() {
		u := reflect.New(t)
		if err = rows.Scan(u.Interface()); err != nil {
			return nil, fmt.Errorf("%s: %s", err.Error(), sql)
		}
		result = append(result, u)
	}

	return result, nil
}

// AddUserIfNotExist ...
func AddUserIfNotExist(db *sql.DB, user GithubUser) error {
	var returnModel GithubUser

	result, err := QuerySQLObject(db, fmt.Sprintf(`SELECT id, name, user_name, token, created_at FROM github_users WHERE user_name = %d;`, user.UserName), returnModel)
	if err != nil {
		log.Println(err)
	}
	if returnModel, ok := result.Interface().(*GithubUser); ok && returnModel.UserName != "" {
		log.Printf("%s already added at %s\n", returnModel.UserName, returnModel.CreatedAt)
	} else {
		_, err = db.Exec(
			"INSERT INTO github_users (name, user_name, token) VALUES (?, ?, ?);",
			user.Name,
			user.UserName,
			user.Token,
		)

		if err != nil {
			log.Println(err)
		} else {
			log.Printf("%s added at %s\n", returnModel.UserName, time.Now())
		}
	}

	return err
}

// StoreTelegramMessage ...
func StoreTelegramMessage(db *sql.DB, message TelegramMessage) error {
	_, err := db.Exec(
		"INSERT INTO telegram_messages (user_id, user_name, message, created_at) VALUES (?, ?, ?, ?);",
		message.UserID,
		message.UserName,
		message.Message,
		message.Date)

	if err != nil {
		return err
	}

	return nil
}
