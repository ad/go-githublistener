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

// TelegramMessage ...
type TelegramMessage struct {
	ID       int
	UserID   int
	UserName string
	Message  string
	Date     time.Time
}

// GithubUser ...
type GithubUser struct {
	ID             int64     `sql:"id"`
	Name           string    `sql:"name"`
	UserName       string    `sql:"user_name"`
	TelegramUserID string    `sql:"telegram_user_id"`
	Token          string    `sql:"token"`
	CreatedAt      time.Time `sql:"created_at"`
}

// GithubRepo ...
type GithubRepo struct {
	ID        int64     `sql:"id"`
	Name      string    `sql:"name"`
	RepoName  string    `sql:"repo_name"`
	CreatedAt time.Time `sql:"created_at"`
}

// UserRepo ...
type UserRepo struct {
	ID        int64     `sql:"id"`
	Name      int       `sql:"user_id"`
	RepoName  int       `sql:"repo_id"`
	CreatedAt time.Time `sql:"created_at"`
	PushedAt  time.Time `sql:"pushed_at"`
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
		"name" text NOT NULL,
		"user_name" text NOT NULL,
		"token" text NOT NULL,
		"telegram_user_id" INTEGER NOT NULL,
		"created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT "github_users_user_name" UNIQUE ("user_name") ON CONFLICT IGNORE
	  );`)
	if err != nil {
		fmt.Println(err)
	}

	err = ExecSQL(db, `CREATE TABLE IF NOT EXISTS "github_repos" (
		"id" INTEGER PRIMARY KEY AUTOINCREMENT,
		"name" text NOT NULL,
		"repo_name" text NOT NULL,
		"created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT "github_repos_repo_name" UNIQUE ("repo_name") ON CONFLICT IGNORE
	  );`)
	if err != nil {
		fmt.Println(err)
	}

	err = ExecSQL(db, `CREATE TABLE IF NOT EXISTS "users_repos" (
		"id" INTEGER PRIMARY KEY AUTOINCREMENT,
		"user_id" INTEGER NOT NULL,
		"repo_id" INTEGER NOT NULL,
		"created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
		"pushed_at" timestamp,
		CONSTRAINT "repos_user_id" FOREIGN KEY ("user_id") REFERENCES "github_users" ("id"),
		CONSTRAINT "repos_repo_id" FOREIGN KEY ("repo_id") REFERENCES "github_repos" ("id"),
		CONSTRAINT "repos_repo_id_user_id" UNIQUE ("user_id", "repo_id") ON CONFLICT IGNORE
	  );`)
	if err != nil {
		fmt.Println(err)
	}

	// CREATE INDEX IF NOT EXISTS "messages_user_id"
	// ON "messages" (
	//   "user_id" ASC
	// );

	// var returnModel TelegramMessage

	// result, err := QuerySQLList(db, `select * from telegram_messages order by id desc limit 0, 5;`, returnModel)
	// if err != nil {
	// 	log.Println(err)
	// }
	// for _, item := range result {
	// 	if returnModel, ok := item.Interface().(*TelegramMessage); ok {
	// 		// FIXME: some problem with time.Time :(
	// 		log.Printf("%s: %s [%d] %s", returnModel.Date, returnModel.UserName, returnModel.UserID, returnModel.Message)
	// 	}
	// }

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
func AddUserIfNotExist(db *sql.DB, user *GithubUser) error {
	var returnModel GithubUser

	result, err := QuerySQLObject(db, fmt.Sprintf(`SELECT * FROM github_users WHERE user_name = '%s';`, user.UserName), returnModel)
	if err != nil {
		return err
	}
	if returnModel, ok := result.Interface().(*GithubUser); ok && returnModel.UserName != "" {
		user.ID = returnModel.ID
		return fmt.Errorf("%s already added at %s\n", returnModel.UserName, returnModel.CreatedAt)
	}

	res, err := db.Exec(
		"INSERT INTO github_users (name, user_name, token, telegram_user_id) VALUES (?, ?, ?, ?);",
		user.Name,
		user.UserName,
		user.Token,
		user.TelegramUserID,
	)

	if err != nil {
		return err
	}

	user.ID, _ = res.LastInsertId()
	log.Printf("%s (%d) added at %s\n", user.UserName, user.ID, time.Now())

	return nil
}

// AddRepoIfNotExist ...
func AddRepoIfNotExist(db *sql.DB, repo *GithubRepo) error {
	var returnModel GithubRepo

	result, err := QuerySQLObject(db, fmt.Sprintf(`SELECT * FROM github_repos WHERE repo_name = '%s';`, repo.RepoName), returnModel)
	if err != nil {
		return err
	}
	if returnModel, ok := result.Interface().(*GithubRepo); ok && returnModel.RepoName != "" {
		repo.ID = returnModel.ID
		return fmt.Errorf("%s already added at %s", returnModel.RepoName, returnModel.CreatedAt)
	}
	res, err := db.Exec(
		"INSERT INTO github_repos (name, repo_name) VALUES (?, ?);",
		repo.Name,
		repo.RepoName,
	)

	if err != nil {
		return err
	}

	repo.ID, _ = res.LastInsertId()
	log.Printf("%s (%d) added at %s\n", repo.RepoName, repo.ID, time.Now())

	return nil
}

// AddRepoLinkIfNotExist ...
func AddRepoLinkIfNotExist(db *sql.DB, user *GithubUser, repo *GithubRepo, pushedAt time.Time) error {
	var returnModel GithubRepo

	result, err := QuerySQLObject(db, fmt.Sprintf(`SELECT * FROM users_repos WHERE user_id = %d AND repo_id = %d;`, user.ID, repo.ID), returnModel)
	if err != nil {
		return err
	}
	if returnModel, ok := result.Interface().(*GithubRepo); ok && returnModel.RepoName != "" {
		return fmt.Errorf("%s already added at %s", returnModel.RepoName, returnModel.CreatedAt)
	}

	res, err := db.Exec(
		"INSERT INTO users_repos (user_id, repo_id, pushed_at) VALUES (?, ?, ?);",
		user.ID,
		repo.ID,
		pushedAt,
	)

	if err != nil {
		return err
	}

	id, _ := res.LastInsertId()

	log.Printf("link %s <-> %s (%d) added at %s\n", user.Name, repo.RepoName, id, time.Now())

	return nil
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
