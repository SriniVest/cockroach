// Copyright 2018 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package xform

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql/opt/optbuilder"
	"github.com/cockroachdb/cockroach/pkg/sql/opt/testutils"
	"github.com/cockroachdb/cockroach/pkg/sql/parser"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/testutils/datadriven"
)

// PhysProps files can be run separately like this:
//   make test PKG=./pkg/sql/opt/xform TESTS="TestPhysicalProps/ordering"
//   make test PKG=./pkg/sql/opt/xform TESTS="TestPhysicalProps/presentation"
//   ...
func TestPhysicalProps(t *testing.T) {
	runDataDrivenTest(t, "testdata/physprops/*")
}

// runDataDrivenTest runs data-driven testcases of the form
//   <command>
//   <SQL statement>
//   ----
//   <expected results>
//
// The supported commands are:
//
//  - exec-ddl
//
//    Runs a SQL DDL statement to build the test catalog. Only a small number
//    of DDL statements are supported, and those not fully.
//
//  - build
//
//    Builds an expression tree from a SQL query and outputs it without any
//    optimizations applied to it.
//
//  - opt
//
//    Builds an expression tree from a SQL query, fully optimizes it using the
//    memo, and then outputs the lowest cost tree.
//
//  - memo
//
//    Builds an expression tree from a SQL query, fully optimizes it using the
//    memo, and then outputs the memo containing the forest of trees.
//
func runDataDrivenTest(t *testing.T, testdataGlob string) {
	paths, err := filepath.Glob(testdataGlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no testfiles found matching: %s", testdataGlob)
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			ctx := context.Background()
			semaCtx := tree.MakeSemaContext(false /* privileged */)
			evalCtx := tree.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())
			catalog := testutils.NewTestCatalog()

			datadriven.RunTest(t, path, func(d *datadriven.TestData) string {
				if d.Cmd == "exec-ddl" {
					return testutils.ExecuteTestDDL(t, d.Input, catalog)
				}

				stmt, err := parser.ParseOne(d.Input)
				if err != nil {
					d.Fatalf(t, "%v", err)
				}

				switch d.Cmd {
				case "build", "opt", "memo":
					// build command disables optimizations, opt enables them.
					var steps OptimizeSteps
					if d.Cmd == "build" {
						steps = OptimizeNone
					} else {
						steps = OptimizeAll
					}
					o := NewOptimizer(&evalCtx, steps)
					b := optbuilder.New(ctx, &semaCtx, &evalCtx, catalog, o.Factory(), stmt)
					root, props, err := b.Build()
					if err != nil {
						d.Fatalf(t, "%v", err)
					}
					exprView := o.Optimize(root, props)

					if d.Cmd == "memo" {
						return fmt.Sprintf("[%d: \"%s\"]\n%s", root, props.String(), o.mem.String())
					}
					return exprView.String()

				default:
					d.Fatalf(t, "unsupported command: %s", d.Cmd)
					return ""
				}
			})
		})
	}
}
