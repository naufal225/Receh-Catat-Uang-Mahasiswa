package usecase

import (
	"context"

	"receh/internal/domain"
)

type CategoryInput struct {
	Name  string        `json:"name"`
	Type  domain.TxType `json:"type"`
	Color string        `json:"color"`
}

type CategoryService struct {
	categories domain.CategoryRepository
	notify     notifier
}

func NewCategoryService(c domain.CategoryRepository, n domain.ChangeNotifier) *CategoryService {
	return &CategoryService{categories: c, notify: notifier{n}}
}

func (s *CategoryService) List(ctx context.Context, userID int64) ([]domain.Category, error) {
	return s.categories.List(ctx, userID)
}

func (s *CategoryService) Create(ctx context.Context, userID int64, in CategoryInput) (*domain.Category, error) {
	defer s.notify.changed(userID)
	name, err := cleanName("name", in.Name, 30, "Nama kategori wajib diisi.")
	if err != nil {
		return nil, err
	}
	if !in.Type.Valid() {
		return nil, domain.Invalid("type", "Pilih pemasukan atau pengeluaran.")
	}
	if err := validColor(in.Color); err != nil {
		return nil, err
	}
	c := &domain.Category{UserID: userID, Name: name, Type: in.Type, Color: in.Color}
	if err := s.categories.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Update changes name and colour. The type stays fixed so past transactions
// never end up under a category of the opposite kind.
func (s *CategoryService) Update(ctx context.Context, userID, id int64, in CategoryInput) (*domain.Category, error) {
	defer s.notify.changed(userID)
	name, err := cleanName("name", in.Name, 30, "Nama kategori wajib diisi.")
	if err != nil {
		return nil, err
	}
	if err := validColor(in.Color); err != nil {
		return nil, err
	}
	c, err := s.categories.ByID(ctx, userID, id)
	if err != nil {
		return nil, notFoundOr(err, "Kategori tidak ditemukan.")
	}
	c.Name, c.Color = name, in.Color
	if err := s.categories.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CategoryService) Delete(ctx context.Context, userID, id int64) error {
	defer s.notify.changed(userID)
	if _, err := s.categories.ByID(ctx, userID, id); err != nil {
		return notFoundOr(err, "Kategori tidak ditemukan.")
	}
	used, err := s.categories.InUse(ctx, id)
	if err != nil {
		return err
	}
	if used {
		return domain.Conflict("CATEGORY_IN_USE", "Kategori ini masih dipakai transaksi atau jadwal rutin, jadi belum bisa dihapus.")
	}
	return s.categories.Delete(ctx, id)
}
