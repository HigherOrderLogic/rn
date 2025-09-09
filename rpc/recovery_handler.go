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

package rpc

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	grpc "google.golang.org/grpc"
	"unstable.build/go-tui/debug"
)

// UnaryReportRecoveryInterceptor implements a grpc.UnaryServerInterceptor
// that recovers and logs panics.
func UnaryReportRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (ret any, err error) {
		ok, reportname, captureErr := debug.CapturePanicReportWith(
			debug.ReportsDir, debug.Package, debug.Tag, func() {
				ret, err = handler(ctx, req)
			})
		if ok {
			return ret, err
		}
		if captureErr != nil {
			panic(fmt.Sprintf("capture panic report: error capturing: %v", captureErr))
		}
		err = fmt.Errorf("grpc goroutine panic: report: %s", reportname)
		log.Panicf("%v", err)
		return
	}
}

// StreamReportRecoveryInterceptor implements a grpc.UnaryServerInterceptor
// that recovers and logs panics.
func StreamReportRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream,
		info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		ok, reportname, captureErr := debug.CapturePanicReportWith(
			debug.ReportsDir, debug.Package, debug.Tag, func() {
				err = handler(srv, stream)
			})
		if ok {
			return err
		}
		if captureErr != nil {
			panic(fmt.Sprintf("capture panic report: error capturing: %v", captureErr))
		}
		err = fmt.Errorf("grpc goroutine panic: report: %s", reportname)
		log.Panicf("%v", err)
		return
	}
}
