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

package component

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unstablebuild/rune-go-sdk/component"
	"github.com/unstablebuild/rune-go-sdk/term"
)

func newTestInterrupter() (term.Interrupter, chan struct{}) {
	ch := make(chan struct{})
	interrupter := term.FuncInterrupter(func(context.Context) error {
		ch <- struct{}{}
		return nil
	})
	return interrupter, ch
}

func TestAnimation(t *testing.T) {
	t.Parallel()
	suite := []struct {
		desc         string
		newAnimation func(*testing.T, term.Interrupter, []string, []int, int) *Animation
	}{
		{"NewAnimation constructor",
			func(t *testing.T, interrupter term.Interrupter, frames []string, sequence []int, fps int) *Animation {
				return NewAnimation(interrupter, frames, sequence, fps)
			}},
		{"encoded and decoded animation",
			func(t *testing.T, interrupter term.Interrupter, frames []string, sequence []int, fps int) *Animation {
				a := NewAnimation(interrupter, frames, sequence, fps)
				defer a.Close()

				raw := EncodeAnimation(a, 8, 4)
				ret, err := DecodeAnimation(raw, fps, interrupter)
				require.NoError(t, err)
				return ret
			}},
		{"non stringer encoded and decoded animation",
			func(t *testing.T, interrupter term.Interrupter, frames []string, sequence []int, fps int) *Animation {
				components := make([]component.WithAttributes, len(frames))
				for i, frame := range frames {
					components[i] = noStringerString{component.NewStringWithConfig(frame, component.StringConfig{
						Alignment: component.AlignmentCentered,
					})}
				}
				a := new(Animation)
				a.InitWithComponents(context.Background(), interrupter, components, sequence, fps)
				defer a.Close()

				raw := EncodeAnimation(a, 8, 4)
				ret, err := DecodeAnimation(raw, fps, interrupter)
				require.NoError(t, err)
				return ret
			}},
	}
	for _, test := range suite {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			t.Run("no frames", func(t *testing.T) {
				t.Parallel()
				fps := 30
				interrupter, _ := newTestInterrupter()

				frames := []string{}
				sequence := []int{}
				expected := "        \n        \n        \n        "

				// sut
				c := NewAnimation(interrupter, frames, sequence, fps)
				c.Resize(8, 4)

				for i := 0; i < 30; i++ {
					w := term.NewStringWriter(8, 4)
					c.Draw(w)

					require.NoError(t, w.Flush())
					assert.Equal(t, expected, w.String())
				}

				require.NoError(t, c.Close())
			})

			t.Run("single static frame", func(t *testing.T) {
				t.Parallel()
				fps := 30
				interrupter, ch := newTestInterrupter()

				frames := []string{"1111    \n1111    \n    @@@@\n    @@@@"}
				sequence := []int{0}
				expected := "1111    \n1111    \n    @@@@\n    @@@@"

				// sut
				c := NewAnimation(interrupter, frames, sequence, fps)
				c.Resize(8, 4)

				for i := 0; i < 30; i++ {
					w := term.NewStringWriter(8, 4)
					<-ch
					c.Draw(w)

					require.NoError(t, w.Flush())
					assert.Equal(t, expected, w.String())
				}

				require.NoError(t, c.Close())
			})

			t.Run("multiple frames, repeated or not", func(t *testing.T) {
				t.Parallel()
				fps := 30
				interrupter, ch := newTestInterrupter()

				frames := []string{
					"0000    \n0000    \n    0000\n    0000",
					"1111    \n1111    \n    1111\n    1111",
				}
				sequence := []int{0, 0, 1}

				// sut
				c := test.newAnimation(t, interrupter, frames, sequence, fps)
				c.Resize(8, 4)

				for i := 0; i < 30; i++ {
					w := term.NewStringWriter(8, 4)
					<-ch
					c.Draw(w)
					var expected string
					switch i % 3 {
					case 0:
						expected = "0000    \n0000    \n    0000\n    0000"
					case 1:
						expected = "0000    \n0000    \n    0000\n    0000"
					case 2:
						expected = "1111    \n1111    \n    1111\n    1111"
					default:
						panic("hmmm")
					}

					require.NoError(t, w.Flush())
					assert.Equal(t, expected, w.String())
				}

				require.NoError(t, c.Close())
			})
		})
	}
}

type noStringerString struct {
	str component.String
}

func (n noStringerString) Draw(w term.Writer) {
	n.str.Draw(w)
}
func (n noStringerString) SetAttr(attr term.Attributes) term.Attributes {
	return n.str.SetAttr(attr)
}

func (n noStringerString) Resize(width, height int) {
	n.str.Resize(width, height)
}
