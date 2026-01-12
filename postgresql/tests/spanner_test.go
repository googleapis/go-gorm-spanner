package tests_test

import (
	"errors"
	"log"
	"os"
	"time"

	spannerpg "github.com/googleapis/go-gorm-spanner/postgresql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var spannerpgDSN = "projects/emulator-project/instances/test-instance/databases/test-database?autoConfigEmulator=true;dialect=POSTGRESQL;decode_numeric_to_string=true"

func OpenTestConnection(cfg *gorm.Config) (db *gorm.DB, err error) {
	dbDSN := os.Getenv("GORM_DSN")
	switch os.Getenv("GORM_DIALECT") {
	case "spannerpg":
		log.Println("testing spannerpg...")
		if dbDSN == "" {
			dbDSN = spannerpgDSN
		}
		shouldLog := os.Getenv("GORM_LOGGING")
		if shouldLog == "true" {
			newLogger := logger.New(
				log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
				logger.Config{
					SlowThreshold:             time.Second, // Slow SQL threshold
					LogLevel:                  logger.Info, // Log level
					IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
					ParameterizedQueries:      true,        // Don't include params in the SQL log
					Colorful:                  true,        // Disable color
				},
			)
			cfg.Logger = newLogger
		}
		db, err = gorm.Open(spannerpg.NewWithSpannerConfig(postgres.Config{
			DSN:                  dbDSN,
			PreferSimpleProtocol: true,
		}, spannerpg.SpannerConfig{AutoOrderByPk: true, AutoAddPrimaryKey: true}), cfg)
	default:
		return nil, errors.New("this function can only be used to test Spanner PostgreSQL")
	}

	if err != nil {
		return
	}

	if debug := os.Getenv("DEBUG"); debug == "true" {
		db.Logger = db.Logger.LogMode(logger.Info)
	} else if debug == "false" {
		db.Logger = db.Logger.LogMode(logger.Silent)
	}

	return
}
