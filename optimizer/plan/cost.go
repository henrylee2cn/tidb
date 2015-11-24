// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package plan

// Pre-defined cost factors.
const (
	DefaultRowCount = 10000
	RowCost         = 1.0
	IndexCost       = 2.0
	SortCost        = 2.0
)

// CostEstimator estimates the cost of a plan.
type costEstimator struct {
}

// Enter implements Visitor Enter interface.
func (c *costEstimator) Enter(p Plan) (Plan, bool) {
	return p, false
}

// Leave implements Visitor Leave interface.
func (c *costEstimator) Leave(p Plan) (Plan, bool) {
	switch v := p.(type) {
	case *IndexScan:
		c.indexScan(v)
	}
	return p, true
}

func (c *costEstimator) indexScan(v *IndexScan) {

}

func EstimateCost(p Plan) float64 {
	return 0
}