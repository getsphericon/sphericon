// Command seed loads the committed fixtures into the development database.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Println("Fixtures loaded successfully")
}

func run() error {
	cfg, err := config.Load(config.EnvDevelopment)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer cleanup.Close(sqlDB, "database")

	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}

	if err := fixtures.Seed(context.Background(), sqlDB, cipher, "fixtures"); err != nil {
		return fmt.Errorf("load fixtures: %w", err)
	}
	return nil
}
