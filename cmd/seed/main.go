package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/go-testfixtures/testfixtures/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Println("Fixtures loaded successfully")
}

func run() error {
	cfg, err := config.Load("development")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}

	loader, err := testfixtures.New(
		testfixtures.Database(sqlDB),
		testfixtures.Dialect("postgres"),
		testfixtures.DangerousSkipTestDatabaseCheck(),
		// Template options MUST precede Directory (it renders files eagerly). Same
		// templating + sequence handling as the test loader (internal/testhelper)
		// so dev and test load identical fixtures.
		testfixtures.Template(),
		testfixtures.TemplateFuncs(fixtures.TemplateFuncs(cipher, time.Now())),
		testfixtures.Directory("fixtures"),
		testfixtures.ResetSequencesTo(100000),
	)
	if err != nil {
		return fmt.Errorf("create fixtures loader: %w", err)
	}

	if err := loader.Load(); err != nil {
		return fmt.Errorf("load fixtures: %w", err)
	}
	return nil
}
