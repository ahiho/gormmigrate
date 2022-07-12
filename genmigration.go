package gormmigrate

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"gorm.io/gorm"
)

const migrationFileTemplate = `// nolint
package migrations

import (
	"gorm.io/gorm"

	"github.com/ahiho/gormmigrate"
)

var {{ migrationName }} = &gormmigrate.Migration{
	ID: "{{ migrationID }}",
	Migrate: func(d *gorm.DB) error {
		commands := []string{
			// add custom commands if needed
{{ migrateCommands }}
		}
		return d.Transaction(func(tx *gorm.DB) error {
			var err error
			for _, command := range commands {
				err = tx.Exec(command).Error
				if err != nil {
					return err
				}
			}
			return nil
		})
	},
}
`

type MigrationsStore interface {
	Migrations() []*Migration
	Models() []interface{}
	DB() *gorm.DB
}

type MigrationOption struct {
	DstFolder string
	Store     MigrationsStore
}

type MigrationCommand string

const (
	CommandPrefix   = "migration"
	CommandGenerate = "generate"
	CommandMigrate  = "migrate"
	// TODO: up and down
)

var (
	ErrNotConfigured  = errors.New("migration not configured")
	ErrInvalidOption  = errors.New("invalid migration option")
	ErrInvalidCommand = errors.New("invalid migration command")

	migrationOp *MigrationOption
	db          *gorm.DB
)

func Config(op *MigrationOption) error {
	if op == nil || op.Store == nil {
		return ErrInvalidOption
	}

	migrationOp = op
	if migrationOp.DstFolder == "" {
		migrationOp.DstFolder = "migrations"
	}

	db = op.Store.DB()

	return nil
}

func ExecuteCommand(args []string) error {
	if migrationOp == nil {
		return ErrNotConfigured
	}
	if len(args) == 0 {
		return ErrInvalidCommand
	}
	command := args[0]
	if !isInStringArr([]string{
		CommandGenerate,
		CommandMigrate,
	}, command) {
		return ErrInvalidCommand
	}

	switch command {
	case CommandGenerate:
		if len(args) < 2 {
			return errors.New("name of migrations is required")
		}
		name := args[1]
		models := migrationOp.Store.Models()
		return generateMigrations(
			name,
			models...,
		)
	case CommandMigrate:
		return MigrateDB()
	}
	return nil
}

func MigrateDB() error {
	if migrationOp == nil {
		return ErrNotConfigured
	}
	migrations := migrationOp.Store.Migrations()
	m := New(db, &Options{
		TableName:    "_migrations",
		IDColumnName: "id",
	}, migrations)

	return m.Migrate()
}

func generateMigrations(name string, dst ...interface{}) (err error) {
	tx := db.Begin()
	var statements []string
	err = tx.Callback().Raw().Remove("gorm:raw")
	if err != nil {
		return err
	}
	err = tx.Callback().Raw().Register("gorm:raw", func(tx *gorm.DB) {
		statements = append(statements, tx.Statement.SQL.String())
	})
	if err != nil {
		return err
	}
	err = tx.AutoMigrate(dst...)
	if err != nil {
		return err
	}
	tx.Rollback()
	_ = tx.Callback().Raw().Remove("gorm:raw")
	commands := []string{}
	for _, s := range statements {
		c := fmt.Sprintf("\t\t\t\"%v\",", s)
		commands = append(commands, c)
	}
	migrateCommands := strings.Join(commands, "\n")
	timestamp := time.Now().UTC().Format("20060102150405")

	snakeName := strcase.ToSnake(name)
	camelName := strcase.ToLowerCamel(name)

	migrationID := fmt.Sprintf("%v_%v", timestamp, snakeName)
	migrationFileName := fmt.Sprintf("migrations/%v_%v.go", timestamp, snakeName)
	migrationName := fmt.Sprintf("%v%v", camelName, timestamp)

	content := strings.ReplaceAll(migrationFileTemplate, "{{ migrationName }}", migrationName)

	content = strings.ReplaceAll(content, "{{ migrationID }}", migrationID)
	content = strings.ReplaceAll(content, "{{ migrateCommands }}", migrateCommands)

	// nolint: gosec
	return os.WriteFile(migrationFileName, []byte(content), 0644)
}

func isInStringArr(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}
