// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vm

import (
	"github.com/luxfi/node/vms/types/fee"
)

// gateUserTx refuses every caller — R-Chain accepts no user txs
// (relayer receipts arrive via consensus, not a mempool). Any
// service entry exposing itself as user-callable MUST route through
// this gate so the refusal is explicit, not implicit.
func (vm *VM) gateUserTx() error {
	if vm.feePolicy == nil {
		return fee.ErrChainAcceptsNoUserTxs
	}
	return vm.feePolicy.ValidateFee(0, fee.NoUserTxPolicy{}.FeeAssetID())
}

// FeePolicy exposes the chain's declared fee policy for diagnostics
// and the boot-time Validate gate.
func (vm *VM) FeePolicy() fee.Policy { return vm.feePolicy }
