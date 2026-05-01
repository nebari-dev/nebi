package rbac

import (
	"errors"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"gorm.io/gorm"
)

type casbinRule struct {
	ID    uint   `gorm:"primaryKey;autoIncrement"`
	Ptype string `gorm:"size:100;index:idx_casbin_rule"`
	V0    string `gorm:"size:100;index:idx_casbin_rule"`
	V1    string `gorm:"size:100;index:idx_casbin_rule"`
	V2    string `gorm:"size:100;index:idx_casbin_rule"`
	V3    string `gorm:"size:100;index:idx_casbin_rule"`
	V4    string `gorm:"size:100;index:idx_casbin_rule"`
	V5    string `gorm:"size:100;index:idx_casbin_rule"`
}

func (casbinRule) TableName() string { return "casbin_rule" }

type gormAdapter struct {
	db *gorm.DB
}

func newGormAdapter(db *gorm.DB) (*gormAdapter, error) {
	if err := db.AutoMigrate(&casbinRule{}); err != nil {
		return nil, fmt.Errorf("auto-migrate casbin_rule: %w", err)
	}
	return &gormAdapter{db: db}, nil
}

func ruleToLine(r casbinRule) string {
	parts := []string{r.Ptype, r.V0, r.V1, r.V2, r.V3, r.V4, r.V5}
	last := len(parts)
	for last > 1 && parts[last-1] == "" {
		last--
	}
	return strings.Join(parts[:last], ", ")
}

func lineFromRule(ptype string, rule []string) casbinRule {
	r := casbinRule{Ptype: ptype}
	fields := []*string{&r.V0, &r.V1, &r.V2, &r.V3, &r.V4, &r.V5}
	for i, v := range rule {
		if i >= len(fields) {
			break
		}
		*fields[i] = v
	}
	return r
}

func (a *gormAdapter) LoadPolicy(m model.Model) error {
	var rules []casbinRule
	if err := a.db.Find(&rules).Error; err != nil {
		return err
	}
	for _, r := range rules {
		if err := persist.LoadPolicyLine(ruleToLine(r), m); err != nil {
			return err
		}
	}
	return nil
}

func (a *gormAdapter) SavePolicy(m model.Model) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&casbinRule{}).Error; err != nil {
			return err
		}
		var rules []casbinRule
		for ptype, ast := range m["p"] {
			for _, rule := range ast.Policy {
				rules = append(rules, lineFromRule(ptype, rule))
			}
		}
		for ptype, ast := range m["g"] {
			for _, rule := range ast.Policy {
				rules = append(rules, lineFromRule(ptype, rule))
			}
		}
		if len(rules) == 0 {
			return nil
		}
		return tx.Create(&rules).Error
	})
}

func (a *gormAdapter) AddPolicy(_ string, ptype string, rule []string) error {
	r := lineFromRule(ptype, rule)
	return a.db.Create(&r).Error
}

func (a *gormAdapter) RemovePolicy(_ string, ptype string, rule []string) error {
	q := a.filterQuery(ptype, 0, rule...)
	return q.Delete(&casbinRule{}).Error
}

func (a *gormAdapter) RemoveFilteredPolicy(_ string, ptype string, fieldIndex int, fieldValues ...string) error {
	if fieldIndex < 0 || fieldIndex > 5 {
		return errors.New("rbac: fieldIndex out of range [0,5]")
	}
	q := a.filterQuery(ptype, fieldIndex, fieldValues...)
	return q.Delete(&casbinRule{}).Error
}

func (a *gormAdapter) filterQuery(ptype string, fieldIndex int, fieldValues ...string) *gorm.DB {
	q := a.db.Where("ptype = ?", ptype)
	cols := []string{"v0", "v1", "v2", "v3", "v4", "v5"}
	for i, v := range fieldValues {
		if v == "" {
			continue
		}
		q = q.Where(fmt.Sprintf("%s = ?", cols[fieldIndex+i]), v)
	}
	return q
}
