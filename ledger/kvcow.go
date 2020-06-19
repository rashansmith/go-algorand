// Copyright (C) 2019-2020 Algorand, Inc.
// This file is part of go-algorand
//
// go-algorand is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// go-algorand is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with go-algorand.  If not, see <https://www.gnu.org/licenses/>.

package ledger

import (
	"github.com/algorand/go-algorand/data/basics"
)

type keyValueCow struct {
	delta basics.StateDelta
}

func makeKeyValueCow() (kvc keyValueCow) {
	kvc.delta = make(basics.StateDelta)
	return
}

func (kvc *keyValueCow) clear() {
	kvc.delta = make(basics.StateDelta)
}

func (kvc *keyValueCow) read(key string) (hitCow bool, value basics.TealValue, ok bool) {
	// If the value for the key has been modified in the delta,
	// then return the modified value.
	valueDelta, ok := kvc.delta[key]
	if ok {
		value, ok := valueDelta.ToTealValue()
		return true, value, ok
	}

	// Otherwise, return the value from the underlying key/value.
	return false, basics.TealValue{}, false
}

func (kvc *keyValueCow) write(key string, value basics.TealValue) {
	kvc.delta[key] = value.ToValueDelta()
}

func (kvc *keyValueCow) del(key string) {
	kvc.delta[key] = basics.ValueDelta{
		Action: basics.DeleteAction,
	}
}
