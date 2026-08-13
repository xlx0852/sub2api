package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type RedeemCode = domain.RedeemCode
func GenerateRedeemCode() (string, error) { return domain.GenerateRedeemCode() }
