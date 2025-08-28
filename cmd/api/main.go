package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gabrielgcmr/sonnda-api/internal/database"
	"github.com/gabrielgcmr/sonnda-api/internal/middleware"
	"github.com/gabrielgcmr/sonnda-api/internal/patient"
	"github.com/gabrielgcmr/sonnda-api/pkg/validation"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1) Conecta ao DB (pode panicar se não conseguir, antes de subir o servidor)
	database.Connect()
	database.DB.AutoMigrate(&patient.Patient{})

	// 2) Inicializa o validator (com traduções, etc)
	if err := validation.Init(); err != nil {
		log.Fatalf("Erro ao iniciar validador: %v", err)
	}

	// 3) Verifica variáveis críticas
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET não está definido")
	}

	// 4) Cria o router Gin
	r := gin.Default()
	r.Use(middleware.SetupCors())

	// 5) Registra as rotas de paciente
	patient.Routes(r)

	// 6) Lê a porta do ambiente ou usa 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf(":%s", port)
	log.Printf("🚀 API running on %s", addr)

	// 7) Inicia o servidor na porta correta
	r.Run("0.0.0.0:" + port) //

}
