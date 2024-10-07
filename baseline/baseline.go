package paac

import (
	"github.com/casbin/casbin/v2"
)

// Wrapper for a SyncedEnforcer, to provide a standardised interface.
// Ended up not being necessary
type BaselineEnforcer struct {
	Enforcer *casbin.SyncedEnforcer
}

func NewBaselineEnforcer(params ...interface{}) (*BaselineEnforcer, error) {
	e := &BaselineEnforcer{}
	var err error
	e.Enforcer, err = casbin.NewSyncedEnforcer(params...)
	return e, err
}

func (e *BaselineEnforcer) Enforce(rvals ...interface{}) (bool, error) {
	return e.Enforcer.Enforce(rvals...)
}

func (e *BaselineEnforcer) EnableAcceptJsonRequest(acceptJsonRequest bool) {
	e.Enforcer.EnableAcceptJsonRequest(acceptJsonRequest)
}

func (e *BaselineEnforcer) AddPolicy(params ...interface{}) (bool, error) {
	return e.Enforcer.AddPolicy(params...)
}
