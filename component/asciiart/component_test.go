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

package asciiart

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"testing"

	"github.com/unstablebuild/rune-go-sdk/component/comptest"
	"github.com/unstablebuild/rune-go-sdk/term"
)

//go:embed test/logo.png
var logo []byte

func TestNewComponent(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(logo))
	if err != nil {
		panic(fmt.Errorf("png decode: %v", err))
	}

	o := NewComponent(img, DefaultConfig())

	w := term.NewStringWriter(60, 20)

	tests := []comptest.TestCase{
		{
			func() { o.Resize(30, 15) }, `
              6                                             
             22                                             
         -a   !00 11                                        
       +     0 .! 00ac                                      
       +0a= ??    !!bc                                      
       +?bb  !!;  a!cc                                      
       +!cc  .+;; c;;                                       
       =!;c  -b:;                                           
       =a:c  -c+: c++                                       
       =b;;  .;++ a;=+                                      
       =c:;  .==   ;:=.                                     
        ;:::      ::==                                      
         ++::::::::=*                                       
           ==******                                         
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            `,
		}, {
			func() { o.Resize(60, 20) }, `
                                                            
                           22                               
                           11112    4                       
                   aaac      ;0000  2111!                   
                          01   !!?  1000aaa                 
               1111a     ???        ????bbb                 
               0000aa    ?!!!!      ?!!!ccc                 
               ????bbb     !!!!ca   aaaaccc                 
               ?!!?bcb     ++ca;;;  aa;;;;                  
               !!!!ccb     ++aa;;;  ;;;                     
               !aaac;c     bbcb:::  :+:=                    
               aaaa;;c     cccc++:  cc+++++                 
               bbbb;;;     ;;;c+++  -;;;:+++                
               cccc:;;     ;;===+     ;;;====               
               ;;cc;;;     ====       ;;:====               
                ;;;:;:::            ::::*===                
                 ::+++::::::::::::::::****+                 
                    +++=====+::+*=******                    
                         +=*******+                         
                                                            `,
		}, {
			func() { o.Resize(10, 5) }, `
    2                                                       
  +=? !                                                     
  =c-;                                                      
  =;. ;.                                                    
    **                                                      
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            
                                                            `,
		},
	}

	comptest.TestComponent(t, o, w, tests)
}
