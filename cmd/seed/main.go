package main

import (
	"database/sql"
	"log"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/go-testfixtures/testfixtures/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg, err := config.Load("development")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("init cipher: %v", err)
	}

	loader, err := testfixtures.New(
		testfixtures.Database(sqlDB),
		testfixtures.Dialect("postgres"),
		testfixtures.DangerousSkipTestDatabaseCheck(),
		// Template options MUST precede Directory (it renders files eagerly). Same
		// templating + sequence handling as the test loader (internal/testhelper)
		// so dev and test load identical fixtures.
		testfixtures.Template(),
		testfixtures.TemplateFuncs(fixtures.TemplateFuncs(cipher)),
		testfixtures.Directory("fixtures"),
		testfixtures.ResetSequencesTo(100000),
	)
	if err != nil {
		log.Fatalf("create fixtures loader: %v", err)
	}

	if err := loader.Load(); err != nil {
		log.Fatalf("load fixtures: %v", err)
	}

	log.Println("Fixtures loaded successfully")
}
