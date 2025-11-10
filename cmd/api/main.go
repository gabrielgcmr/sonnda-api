package main

import (
	"log"
	"os"

	"time"

	"sonnda-api/internal/auth"
	"sonnda-api/internal/core/jwt"
	"sonnda-api/internal/core/model"
	"sonnda-api/internal/database"
	"sonnda-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

func main() {

	//conectar db
	database.Connect()
	db := database.DB

	//montar o gin e rotas
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 🌐 Aplica o middleware de CORS
	r.Use(middleware.SetupCors())

	//rotas de saúde
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": "1.0.0",
		})
	})

	//config
	jwtMgr := jwt.NewJWTManager(
		os.Getenv("JWT_SECRET"),
		"sonnda-api",
		24*time.Hour,
	)

	//routes
	apiV1 := r.Group("/api/v1")
	auth.AuthRoutes(apiV1, db, jwtMgr)

	//migrations
	if err := db.AutoMigrate(
		&model.User{},
		&model.PatientProfile{},
	); err != nil {
		log.Fatalf("Erro ao migrar tabela users: %v", err)
	}

	log.Println("🚀 API running at http://localhost:8080")
	r.Run(":8080")
}
