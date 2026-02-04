// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

//nolint:unused
package cell

import (
	"context"
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"

	"github.com/unstablebuild/rune-go-sdk/term"
)

type logger struct {
	out   *log.Logger
	r     View
	rType string
	w     Editor
	wType string
}

func newLogger(r View, w Editor, out *log.Logger) *logger {
	lg := &logger{
		out:   out,
		r:     r,
		rType: reflect.TypeOf(r).Elem().String(),
		w:     w,
		wType: reflect.TypeOf(w).Elem().String(),
	}
	return lg
}

func (l *logger) rFields(method string) log.Fields {
	return log.Fields{
		"address": fmt.Sprintf("%p", l.r),
		"type":    l.rType,
		"method":  method,
	}
}

func (l *logger) wFields(method string) log.Fields {
	return log.Fields{
		"address": fmt.Sprintf("%p", l.w),
		"type":    l.wType,
		"method":  method,
	}
}

func (l *logger) Edit(ctx context.Context, start, end term.Coordinates, str string) (
	from, to term.Coordinates, old string,
) {
	fields := l.wFields("update")
	fields["start"] = start
	fields["end"] = end
	fields["string"] = str
	fields["length"] = len(str)

	from, to, old = l.w.Edit(ctx, start, end, str)

	fields["from"] = from
	fields["to"] = to
	fields["old"] = old

	l.out.WithFields(fields).Trace()
	return
}

func (l *logger) Rows() (rows int) {
	fields := l.rFields("rows")

	rows = l.r.Rows()
	fields["rows"] = rows

	l.out.WithFields(fields).Trace()

	return
}

func (l *logger) Columns(row int) (cols int) {
	fields := l.rFields("columns")
	fields["row"] = row

	cols = l.r.Columns(row)

	fields["cols"] = cols

	l.out.WithFields(fields).Trace()

	return
}

func (l *logger) Cell(p term.Coordinates) (
	c term.Cell, ok bool,
) {
	fields := l.rFields("cell")
	fields["position"] = p

	c, ok = l.r.Cell(p)

	fields["cell"] = c
	fields["ok"] = ok

	l.out.WithFields(fields).Trace()

	return
}

func (l *logger) RawCells() (cells [][]term.Cell) {
	fields := l.rFields("rawCells")

	cells = l.r.RawCells()

	fields["rows"] = len(cells)

	l.out.WithFields(fields).Trace()

	return
}

func (l *logger) String() (str string) {
	fields := l.rFields("String")

	str = l.r.String()

	fields["string"] = fmt.Sprintf("%.10s", str)
	fields["length"] = len(str)

	l.out.WithFields(fields).Trace()

	return
}
