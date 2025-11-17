package patient

import (
	"context"
	"errors"
	"sonnda-api/internal/core/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	// Operações CRUD básicas
	Create(ctx context.Context, p *model.Patient) error
	Update(ctx context.Context, p *model.Patient) error
	Delete(ctx context.Context, id uint) error

	// Finders
	FindByCPF(ctx context.Context, cpf string) (*model.Patient, error)
	List(ctx context.Context, limit, offset int) ([]model.Patient, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

// Create: Cadastra um novo usuário
func (r *repository) Create(ctx context.Context, p *model.Patient) error {
	query := `
	INSERT INTO patients (
		cpf, cns, full_name, birth_date, gender, race, avatar_url, phone
	)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	RETURNING id, created_at, updated_at;
	`

	return r.db.QueryRow(ctx, query,
		p.CPF,
		p.CNS,
		p.FullName,
		p.BirthDate,
		p.Gender,
		p.Race,
		p.AvatarURL,
		p.Phone,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

}

// Update: Atualiza dados do paciente
func (r *repository) Update(ctx context.Context, p *model.Patient) error {
	query := `
	UPDATE patients
	SET full_name=$1, birth_date=$2, gender=$3, race=$4, avatar_url=$5, phone=$6, updated_at=NOW()
	WHERE id=$7;
	`
	_, err := r.db.Exec(ctx, query,
		p.FullName,
		p.BirthDate,
		p.Gender,
		p.Race,
		p.AvatarURL,
		p.Phone,
		p.ID,
	)

	return err
}

// Delete remove paciente (soft delete se configurado)
func (r *repository) Delete(ctx context.Context, id uint) error {
	_, err := r.db.Exec(ctx, "DELETE FROM patients WHERE id=$1", id)
	return err
}

// FindByUserID busca paciente por user_id
func (r *repository) FindByCPF(ctx context.Context, cpf string) (*model.Patient, error) {
	query := `
        SELECT id, cpf, cns, full_name, birth_date, gender, race, avatar_url, phone, created_at, updated_at
        FROM patients
        WHERE cpf=$1
        LIMIT 1;
    `
	var p model.Patient
	err := r.db.QueryRow(ctx, query, cpf).Scan(
		&p.ID,
		&p.CPF,
		&p.CNS,
		&p.FullName,
		&p.BirthDate,
		&p.Gender,
		&p.Race,
		&p.AvatarURL,
		&p.Phone,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// List retorna lista de pacientes com paginação
func (r *repository) List(ctx context.Context, limit, offset int) ([]model.Patient, error) {
	query := `
	SELECT id, cpf, cns, full_name, birth_date, gender, race, avatar_url, phone, created_at, updated_at
	FROM patients
	ORDER BY id DESC
	LIMIT $1 OFFSET $2;
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Patient

	for rows.Next() {
		var p model.Patient
		if err := rows.Scan(&p.ID, &p.CPF, &p.CNS, &p.FullName,
			&p.BirthDate, &p.Gender, &p.Race, &p.AvatarURL,
			&p.Phone, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}

	return out, nil

}

// FindAuthorizations retorna todas as autorizações do paciente

// FindAuthorizationByUser busca autorização específica

// CreateAuthorization cria nova autorização

// UpdateAuthorization atualiza autorização

// CreateMedicalRecord cria registro médico

// FindMedicalRecords retorna histórico médico do paciente
