package admin

type Service interface {
	// Define os métodos que o serviço admin deve implementar
}
type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}
