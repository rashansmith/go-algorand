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

type baseGetter func(key string) (value basics.TealValue, ok bool)

type keyValueCow struct {
	delta basics.StateDelta
}

func makeKeyValueCow() (kvc keyValueCow) {
	kvc.delta = make(basics.StateDelta)
	return
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

func (kvc *keyValueCow) write(key string, value basics.TealValue, bv basics.TealValue, bvok bool) {
	// If the value being written is identical to the underlying key/value,
	// then ensure there is no delta entry for the key.
	if bvok && value == bv {
		delete(kvc.delta, key)
	} else {
		// Otherwise, update the delta with the new value.
		kvc.delta[key] = value.ToValueDelta()
	}
}

func (kvc *keyValueCow) del(key string, bvok bool) {
	if bvok {
		// If the key already exists in the underlying key/value,
		// update the delta to indicate that the value was deleted.
		kvc.delta[key] = basics.ValueDelta{
			Action: basics.DeleteAction,
		}
	} else {
		// Since the key didn't exist in the underlying key/value,
		// don't include a delta entry for its deletion.
		delete(kvc.delta, key)
	}
}
