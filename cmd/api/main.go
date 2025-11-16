package main

import (
	"context"
	"log"
	"os"

	"time"

	"sonnda-api/internal/auth"
	"sonnda-api/internal/core/jwt"
	"sonnda-api/internal/core/model"
	"sonnda-api/internal/database"
	"sonnda-api/internal/exam"
	"sonnda-api/internal/middleware"
	"sonnda-api/internal/patient"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	//Contexto raiz
	ctx := context.Background()

	//carregar .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env não carregado automaticamente")
	}

	//conectar db
	database.Connect()
	db := database.DB

	//GCS
	storage, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("erro ao criar storage client: %v", err)
	}

	//montar o gin e rotas
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 🌐 Aplica o middleware de CORS
	r.Use(middleware.SetupCors())

	//config
	jwtMgr := jwt.NewJWTManager(
		os.Getenv("JWT_SECRET"),
		"sonnda-api",
		24*time.Hour,
	)

	//routes
	apiV1 := r.Group("/api/v1")

	//rotas de saúde
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": "1.0.0",
		})
	})

	//Modules
	//Auth
	authModule := auth.NewModule(db, jwtMgr)
	authModule.SetupRoutes(apiV1)

	//Patient
	patientModule := patient.NewModule(db)
	patientModule.SetupRoutes(apiV1)

	//Exam
	examModule := exam.NewModule(storage, "sonnda.firebasestorage.app")
	examModule.SetupRoutes(apiV1)

	// FUTUROS:
	// doctorHandler := doctor.Build(db)
	// doctor.Routes(api, doctorHandler)

	//migrations
	if err := db.AutoMigrate(
		&model.User{},
		&model.Patient{},
	); err != nil {
		log.Fatalf("Erro ao migrar tabela users: %v", err)
	}

	log.Println("🚀 API running at http://localhost:8080")
	r.Run(":8080")
}
