package service

import (
	"context"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
)

type CategoryInput struct {
	Name  string       `json:"name"`
	Type  model.TxType `json:"type"`
	Color string       `json:"color"`
}

type CategoryService struct {
	categories model.CategoryRepository
	notify     notifier
}

func NewCategoryService(c model.CategoryRepository, n domain.ChangeNotifier) *CategoryService {
	return &CategoryService{categories: c, notify: notifier{n}}
}

func (s *CategoryService) List(ctx context.Context, orgID int64) ([]model.Category, error) {
	return s.categories.List(ctx, orgID)
}

func (s *CategoryService) Create(ctx context.Context, orgID int64, in CategoryInput) (*model.Category, error) {
	defer s.notify.changed(orgID)
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
	c := &model.Category{OrgID: orgID, Name: name, Type: in.Type, Color: in.Color}
	if err := s.categories.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Update changes name and colour. The type stays fixed so past transactions
// never end up under a category of the opposite kind.
func (s *CategoryService) Update(ctx context.Context, orgID, id int64, in CategoryInput) (*model.Category, error) {
	defer s.notify.changed(orgID)
	name, err := cleanName("name", in.Name, 30, "Nama kategori wajib diisi.")
	if err != nil {
		return nil, err
	}
	if err := validColor(in.Color); err != nil {
		return nil, err
	}
	c, err := s.categories.ByID(ctx, orgID, id)
	if err != nil {
		return nil, notFoundOr(err, "Kategori tidak ditemukan.")
	}
	c.Name, c.Color = name, in.Color
	if err := s.categories.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CategoryService) Delete(ctx context.Context, orgID, id int64) error {
	defer s.notify.changed(orgID)
	if _, err := s.categories.ByID(ctx, orgID, id); err != nil {
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

// DefaultCategories is what every new account starts with. Each type gets its
// slots in palette order, so a set of categories stays colourblind-safe.
func DefaultCategories() []model.Category {
	return []model.Category{
		{Name: "Makan", Type: model.TxExpense, Color: "c1"},
		{Name: "Minum", Type: model.TxExpense, Color: "c2"},
		{Name: "Transportasi", Type: model.TxExpense, Color: "c3"},
		{Name: "Listrik", Type: model.TxExpense, Color: "c4"},
		{Name: "Pulsa & Internet", Type: model.TxExpense, Color: "c5"},
		{Name: "Kuliah", Type: model.TxExpense, Color: "c6"},
		{Name: "Langganan", Type: model.TxExpense, Color: "c7"},
		{Name: "Hiburan", Type: model.TxExpense, Color: "c8"},
		{Name: FeeCategoryName, Type: model.TxExpense, Color: "c0"},
		{Name: "Lainnya", Type: model.TxExpense, Color: "c0"},
		{Name: "Uang Saku", Type: model.TxIncome, Color: "c1"},
		{Name: "Beasiswa", Type: model.TxIncome, Color: "c2"},
		{Name: "Kerja Sambilan", Type: model.TxIncome, Color: "c3"},
		{Name: "Freelance", Type: model.TxIncome, Color: "c4"},
		{Name: "Lainnya", Type: model.TxIncome, Color: "c0"},
	}
}
