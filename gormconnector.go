package gormconnector

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/gpa"
	"github.com/lemmego/gpagorm"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type Provider struct {
	UseGPA     bool
	config     gpa.Config
	gormConfig *gorm.Config
	appConfig  config.Configuration
	sqlDB      *sql.DB
}

func (g *Provider) WithGPAConfig(config gpa.Config) *Provider {
	g.config = config
	return g
}

func (g *Provider) WithGORMConfig(config *gorm.Config) *Provider {
	g.gormConfig = config
	return g
}

func (g *Provider) AddCommands() []app.Command {
	return []app.Command{
		genGormModelCmd,
		genGormRepoCmd,
	}
}

func (g *Provider) GetSQLDb() *sql.DB {
	return g.sqlDB
}

func (g *Provider) Provide(a app.App) error {
	g.appConfig = a.Config()
	dbConfig := sqlConfig()
	if g.config.Host != "" {
		dbConfig = g.config
	}

	if g.UseGPA {
		provider, err := gpagorm.NewProvider(dbConfig)
		if err != nil {
			panic(err)
		}
		g.sqlDB = provider.DB().(*sql.DB)
		gpa.RegisterDefault(provider)
		a.AddService(provider)
	} else {
		db, err := NewGormConnection(dbConfig)
		if err != nil {
			panic(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			panic(err)
		}
		g.sqlDB = sqlDB
		a.AddService(db)
	}

	return nil
}

func (g *Provider) Shutdown(ctx context.Context) error {
	if g.UseGPA {
		gpa.Registry().RemoveAll()
	}
	if g.sqlDB != nil {
		return g.sqlDB.Close()
	}
	return nil
}

func NewGormConnection(config gpa.Config) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: false,
		},
	}

	if options, ok := config.Options["gorm"]; ok {
		if gormOpts, ok := options.(map[string]any); ok {
			if logLevel, ok := gormOpts["log_level"].(string); ok {
				switch logLevel {
				case "silent":
					gormConfig.Logger = logger.Default.LogMode(logger.Silent)
				case "error":
					gormConfig.Logger = logger.Default.LogMode(logger.Error)
				case "warn":
					gormConfig.Logger = logger.Default.LogMode(logger.Warn)
				case "info":
					gormConfig.Logger = logger.Default.LogMode(logger.Info)
				}
			}

			if singularTable, ok := gormOpts["singular_table"].(bool); ok {
				gormConfig.NamingStrategy = schema.NamingStrategy{
					SingularTable: singularTable,
				}
			}
		}
	}

	var dialector gorm.Dialector

	switch strings.ToLower(config.Driver) {
	case "postgres", "postgresql":
		dialector = postgres.Open(buildPostgresDSN(config))
	case "mysql":
		dialector = mysql.Open(buildMySQLDSN(config))
	case "sqlite", "sqlite3":
		dialector = sqlite.Open(config.Database)
	case "sqlserver", "mssql":
		dialector = sqlserver.Open(buildSQLServerDSN(config))
	default:
		return nil, fmt.Errorf("unsupported driver: %s", config.Driver)
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if config.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	}
	if config.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	}

	return db, nil
}

func sqlConfig(connName ...string) gpa.Config {
	name := "default"
	if len(connName) > 0 && connName[0] != "" {
		name = connName[0]
	}

	defaultConnection := config.Get(fmt.Sprintf("sql.%s", name))
	connection := config.Get(fmt.Sprintf("sql.connections.%s", defaultConnection)).(config.M)
	driver := connection.String("driver")
	database := connection.String("database")

	if database == "" || driver == "" {
		panic("database: database and driver must be present")
	}

	dbConfig := gpa.Config{
		Driver:   driver,
		Database: database,
	}

	if driver != "sqlite" {
		dbConfig.Host = config.Get(fmt.Sprintf("sql.connections.%s.host", defaultConnection)).(string)
		dbConfig.Port = config.Get(fmt.Sprintf("sql.connections.%s.port", defaultConnection)).(int)
		dbConfig.Username = config.Get(fmt.Sprintf("sql.connections.%s.user", defaultConnection)).(string)
		dbConfig.Password = config.Get(fmt.Sprintf("sql.connections.%s.password", defaultConnection)).(string)
		dbConfig.Options = config.Get(fmt.Sprintf("sql.connections.%s.options", defaultConnection)).(config.M)
	}

	return dbConfig
}

// =====================================
// Helper Functions
// =====================================

// buildPostgresDSN builds a PostgreSQL DSN
func buildPostgresDSN(config gpa.Config) string {
	if config.ConnectionURL != "" {
		return config.ConnectionURL
	}

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database)

	if config.SSL.Enabled {
		dsn += " sslmode=" + config.SSL.Mode
		if config.SSL.CertFile != "" {
			dsn += " sslcert=" + config.SSL.CertFile
		}
		if config.SSL.KeyFile != "" {
			dsn += " sslkey=" + config.SSL.KeyFile
		}
		if config.SSL.CAFile != "" {
			dsn += " sslrootcert=" + config.SSL.CAFile
		}
	} else {
		dsn += " sslmode=disable"
	}

	return dsn
}

// buildMySQLDSN builds a MySQL DSN
func buildMySQLDSN(config gpa.Config) string {
	if config.ConnectionURL != "" {
		return config.ConnectionURL
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.Username, config.Password, config.Host, config.Port, config.Database)

	if config.SSL.Enabled {
		dsn += "&tls=" + config.SSL.Mode
	}

	return dsn
}

// buildSQLServerDSN builds a SQL Server DSN
func buildSQLServerDSN(config gpa.Config) string {
	if config.ConnectionURL != "" {
		return config.ConnectionURL
	}

	return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		config.Username, config.Password, config.Host, config.Port, config.Database)
}

// SupportedDrivers returns the list of supported database drivers
func SupportedDrivers() []string {
	return []string{"postgres", "postgresql", "mysql", "sqlite", "sqlite3", "sqlserver", "mssql"}
}

func Get(a app.App) *gpagorm.Provider {
	return app.Get[*gpagorm.Provider](a)
}
