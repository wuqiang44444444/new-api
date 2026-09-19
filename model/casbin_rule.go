package model

type CasbinRule struct {
	Id    uint   `gorm:"primaryKey;autoIncrement"`
	Ptype string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:1"`
	V0    string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:2"`
	V1    string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:3"`
	V2    string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:4"`
	V3    string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:5"`
	V4    string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:6"`
	V5    string `gorm:"size:100;uniqueIndex:idx_casbin_rule_unique,priority:7"`
}

func (CasbinRule) TableName() string {
	return "casbin_rule"
}
