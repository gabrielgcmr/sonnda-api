package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	// DEBUG: Mostre as variáveis que estão sendo usadas
	log.Printf("🔍 Configuração do Banco:")
	log.Printf("   DB_HOST: %s", host)
	log.Printf("   DB_PORT: %s", port)
	log.Printf("   DB_USER: %s", user)
	log.Printf("   DB_NAME: %s", dbname)

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=America/Sao_Paulo",
		host, port, user, password, dbname,
	)

	log.Printf("🔄 Tentando conectar no banco...")

	var db *gorm.DB
	var err error

	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			log.Printf("⚠️  Tentativa %d/5 falhou: %v", i+1, err)
			time.Sleep(3 * time.Second)
			continue
		}
		break
	}

	if err != nil {
		log.Fatalf("❌ Erro ao conectar no banco após várias tentativas: %v", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db
	log.Println("✅ Banco de dados conectado com sucesso!")

}
