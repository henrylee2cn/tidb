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

package optimizer

import (
	"github.com/pingcap/tidb/optimizer/plan"
	"github.com/pingcap/tidb/ast"
	"github.com/pingcap/tidb/parser/opcode"
	"github.com/pingcap/tidb/model"
	"github.com/pingcap/tidb/context"
)

func refine(ctx context.Context, p plan.Plan) {
	r := refiner{ctx: ctx}
	p.Accept(&r)
}

// refiner tries to build index range, bypass sort, set limit for source plan.
// It prepares the plan for cost estimation.
type refiner struct {
	ctx context.Context
	conditions []ast.ExprNode
	newConditions []ast.ExprNode
	// store index scan plan for sort to use.
	indexScan *plan.IndexScan
}

func (r *refiner) Enter(in plan.Plan) (plan.Plan, bool) {
	switch x := in.(type) {
	case *plan.Filter:
		r.conditions = x.Conditions
	case *plan.IndexScan:
		r.indexScan = x
		r.buildIndexRange(x)
	}
	return in, false
}

func (r *refiner) Leave(in plan.Plan) (plan.Plan, bool) {
	switch x := in.(type) {
	case *plan.IndexScan:
		r.buildIndexRange(x)
	case *plan.Filter:
		x.Conditions = r.conditions
	case *plan.Sort:
		r.sortBypass(x)
	case *plan.Limit:
		x.SetLimit(0)
	}
	return in, true
}

func (r *refiner) sortBypass(p *plan.Sort) {
	if r.indexScan != nil {
		idx := r.indexScan.Index
		if len(p.ByItems) > len(idx.Columns) {
			return
		}
		var desc, asc bool
		for i, val := range p.ByItems {
			if val.Desc {
				desc = true
			} else {
				asc = true
			}
			cn, ok := val.Expr.(*ast.ColumnNameExpr)
			if !ok {
				return
			}
			if idx.Table.L != cn.Refer.Table.Name.L {
				return
			}
			indexColumn := idx.Columns[i]
			if indexColumn.Name.L != cn.Refer.Column.Name.L {
				return
			}
		}
		if desc && asc {
			// mixed descend and ascend order can not bypass.
			return
		} else if desc {
			r.indexScan.Desc = true
		}
		p.Bypass = true
	}
}

func (r *refiner) buildIndexRange(p *plan.IndexScan) {
	rb := rangeBuilder{ctx: r.ctx}
	for i := 0; i < len(p.Index.Columns); i++ {
		checker := conditionChecker{idx: p.Index, columnOffset: i}
		var rangePoints []rangePoint
		var newConditions []ast.ExprNode
		for _, cond := range r.conditions {
			if checker.check(cond) {
				rangePoints = rb.intersection(rangePoints, rb.build(cond))
			} else {
				newConditions = append(newConditions, cond)
			}
		}
		r.conditions = newConditions
		if rangePoints == nil {
			break
		}
		if i == 0 {
			p.Ranges = rb.buildIndexRanges(rangePoints)
		} else {
			p.Ranges = rb.appendIndexRanges(p.Ranges, rangePoints)
		}
	}
}

// conditionChecker checks if this condition can be pushed to index plan.
type conditionChecker struct {
	idx *model.IndexInfo
	// the offset of the indexed column to be checked.
	columnOffset int
}

func (c *conditionChecker) check(condition ast.ExprNode) bool {
	switch x := condition.(type) {
	case *ast.BinaryOperationExpr:
		return c.checkBinaryOperation(x)
	case *ast.ParenthesesExpr:
		return c.check(x.Expr)
	case *ast.UnaryOperationExpr:
		if x.Op != opcode.Not {
			return false
		}
		return c.check(x.V)
	case *ast.PatternInExpr:
		if x.Sel != nil {
			return false
		}
		if !c.check(x.Expr) {
			return false
		}
		for _, val := range x.List {
			if !c.check(val) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *conditionChecker) checkBinaryOperation(b *ast.BinaryOperationExpr) bool {
	switch b.Op {
	case opcode.OrOr:
		return c.check(b.L) && c.check(b.R)
	case opcode.EQ, opcode.NE, opcode.GE, opcode.GT, opcode.LE, opcode.LT:
		if b.L.IsStatic() {
			return c.checkColumnExpr(b.R)
		} else if b.R.IsStatic() {
			return c.checkColumnExpr(b.L)
		}
	}
	return false
}

func (c *conditionChecker) checkColumnExpr(expr ast.ExprNode) bool {
	cn, ok := expr.(*ast.ColumnNameExpr)
	if !ok {
		return false
	}
	if cn.Refer.Table.Name.L != c.idx.Table.L {
		return false
	}
	if cn.Refer.Column.Name.L != c.idx.Columns[c.columnOffset].Name.L {
		return false
	}
	return true
}