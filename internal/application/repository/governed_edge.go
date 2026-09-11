package repository

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *tenantRepository) ApplyGovernedEdgeBinding(ctx context.Context, tenantID uint64, binding types.GovernedEdgeBinding) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "governed_edge_binding").First(&tenant, tenantID).Error; err != nil {
			return err
		}
		if current := tenant.GovernedEdgeBinding; current != nil {
			if *current == binding {
				return nil
			}
			if current.BindingID != binding.BindingID || current.EnterpriseID != binding.EnterpriseID || binding.Revision <= current.Revision {
				return fmt.Errorf("governed Edge binding revision conflicts")
			}
		}
		return tx.Model(&tenant).Update("governed_edge_binding", binding).Error
	})
}
