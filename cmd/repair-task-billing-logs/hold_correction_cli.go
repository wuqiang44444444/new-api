package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func runHeldProjectionCommand(db *gorm.DB, output, input, approvedSHA string, generation int64, actor int, reason string) error {
	if output != "" && input != "" {
		return fmt.Errorf("select plan or apply, not both")
	}
	if output != "" {
		var plan *holdCorrectionPlan
		if err := db.Transaction(func(tx *gorm.DB) error { var err error; plan, err = auditHeldTaskProjection(tx); return err }); err != nil {
			return err
		}
		data, err := common.Marshal(plan)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("plan path must be new and writable")
		}
		if _, err = file.Write(data); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		total := int64(0)
		for _, c := range plan.Candidates {
			if !c.Applied {
				total += c.Quota
			}
		}
		fmt.Printf("candidates=%d unresolved=%d missing_quota=%d plan_sha256=%x\n", len(plan.Candidates), len(plan.Unresolved), total, sha256.Sum256(data))
		return nil
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("review evidence and customer correction/notification reference are required")
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("cannot read approved plan")
	}
	if len(approvedSHA) != 64 || fmt.Sprintf("%x", sha256.Sum256(data)) != approvedSHA {
		return fmt.Errorf("approved plan checksum mismatch")
	}
	var plan holdCorrectionPlan
	if common.Unmarshal(data, &plan) != nil {
		return fmt.Errorf("invalid correction plan")
	}
	var count int
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := requireRepairRoot(tx, actor); err != nil {
			return err
		}
		var err error
		count, err = applyHeldTaskProjection(tx, &plan, generation, actor, reason)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Printf("corrected_logs=%d; funds_unchanged=true; confirmed_versions_unchanged=true; correction_versions_and_customer_notice_required=true\n", count)
	return nil
}

func requireRepairRoot(db *gorm.DB, actorID int) error {
	var actor model.User
	if actorID <= 0 {
		return fmt.Errorf("active root operator is required")
	}
	if err := db.Select("id,role,status").First(&actor, actorID).Error; err != nil {
		return fmt.Errorf("cannot verify root operator")
	}
	if actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
		return fmt.Errorf("active root operator is required")
	}
	return nil
}
