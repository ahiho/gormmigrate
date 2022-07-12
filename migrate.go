package gormmigrate

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	// ErrMissingID is returned when the ID od migration is equal to ""
	ErrMissingID = errors.New("missing ID in migration")
)

// MigrateFunc is the func signature for migrating.
type MigrateFunc func(*gorm.DB) error

// InitSchemaFunc is the func signature for initializing the schemas.
type InitSchemaFunc func(*gorm.DB) error

type Migration struct {
	// ID is the migration identifier. Usually a timestamp like "201601021504".
	ID string
	// Migrate is a function that will br executed while running this migration.
	Migrate MigrateFunc
}

// Options define options for all migrations.
type Options struct {
	// TableName is the migration table.
	TableName string
	// IDColumnName is the name of column where the migration id will be stored.
	IDColumnName string
}

// Migrate represents a collection of all migrations of a database schemas.
type Migrate struct {
	db         *gorm.DB
	options    *Options
	migrations []*Migration
	initSchema InitSchemaFunc
}

// New returns a new Gormigrate.
func New(db *gorm.DB, options *Options, migrations []*Migration) *Migrate {
	return &Migrate{
		db:         db,
		options:    options,
		migrations: migrations,
	}
}

// InitSchema sets a function that is run if no migration is found.
// The idea is preventing to run all migrations when a new clean database
// is being migrating. In this function you should create all tables and
// foreign key necessary to your application.
func (m *Migrate) InitSchema(initSchema InitSchemaFunc) {
	m.initSchema = initSchema
}

// Migrate executes all migrations that did not run yet.
func (m *Migrate) Migrate() error {
	if err := m.createMigrationTableIfNotExists(); err != nil {
		return err
	}

	if m.initSchema != nil && m.isFirstRun() {
		return m.runInitSchema()
	}

	for _, migration := range m.migrations {
		if err := m.runMigration(migration); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrate) runInitSchema() error {
	if err := m.initSchema(m.db); err != nil {
		return err
	}

	for _, migration := range m.migrations {
		if err := m.insertMigration(migration.ID); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrate) runMigration(migration *Migration) error {
	if len(migration.ID) == 0 {
		return ErrMissingID
	}

	run, err := m.migrationDidRun(migration)
	if err != nil {
		return err
	}

	if !run {
		if err := migration.Migrate(m.db); err != nil {
			return err
		}

		if err := m.insertMigration(migration.ID); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrate) createMigrationTableIfNotExists() error {
	exists := m.db.Migrator().HasTable(m.options.TableName)
	if exists {
		return nil
	}

	sql := fmt.Sprintf("CREATE TABLE %s (%s VARCHAR(255) PRIMARY KEY)", m.options.TableName, m.options.IDColumnName)
	tx := m.db.Exec(sql)
	return tx.Error
}

func (m *Migrate) migrationDidRun(mig *Migration) (bool, error) {
	count := 0
	tx := m.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", m.options.TableName, m.options.IDColumnName), mig.ID).Scan(&count)
	return count > 0, tx.Error
}

func (m *Migrate) isFirstRun() bool {
	row := m.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", m.options.TableName))
	var count int
	row.Scan(&count)
	return count == 0
}

func (m *Migrate) insertMigration(id string) error {
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (?)", m.options.TableName, m.options.IDColumnName)
	fmt.Printf("Execute %v with param %v", sql, id)
	tx := m.db.Exec(sql, id)
	return tx.Error
}
