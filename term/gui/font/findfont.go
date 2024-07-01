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
package font

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/unstablebuild/blue/iterator"
	"github.com/unstablebuild/blue/logging"
	"golang.org/x/image/font/sfnt"
	"unstable.build/go-tui/workspace/walkdir"
)

type findFont interface {
	findByFamily(family string) (iterator.Iterator[metadata], error)
	list() (iterator.Iterator[metadata], error)
}

type systemFindFont struct {
	reader walkdir.Reader
}

func (s systemFindFont) findByFamily(family string) (
	iterator.Iterator[metadata], error,
) {
	fonts, err := s.find(matchFamily(family))
	if err != nil {
		return nil, err
	}

	it, isEmpty := iterator.IsEmpty(fonts)
	if isEmpty {
		return nil, fmt.Errorf("font '%s' not found", family)
	}
	return it, nil
}

func (s systemFindFont) list() (iterator.Iterator[metadata], error) {
	fonts, err := s.find(nil)
	if err != nil {
		return nil, err
	}
	return fonts, nil
}

func (s systemFindFont) find(matcher matcher) (
	iterator.Iterator[metadata], error,
) {
	ctx := context.Background()
	var iters []iterator.Iterator[metadata]
	for _, dir := range fontDirs() {
		if info, err := os.Stat(dir); os.IsNotExist(err) || !info.IsDir() {
			continue
		}

		it, err := walkdir.ListFiles(ctx, s.reader, dir)
		if err != nil {
			s.log(log.WarnLevel, "list files in dir '%s': %v", dir, err)
			continue
		}

		validit := iterator.Filter(it, func(path string) bool {
			ext := filepath.Ext(path)
			return strings.EqualFold(ext, ".ttf") ||
				strings.EqualFold(ext, ".ttc") ||
				strings.EqualFold(ext, ".otc") ||
				strings.EqualFold(ext, ".otf")
		})

		metait := iterator.Map(validit, func(path string) []metadata {
			f, err := os.Open(path)
			if err != nil {
				s.log(log.WarnLevel, "open font file '%s': %v", path, err)
				return nil
			}
			defer f.Close()
			m, err := readMetadata(path, f)
			if err != nil {
				s.log(log.WarnLevel, "read font '%s' metadata: %v", path, err)
				return nil
			}
			return m
		})

		unsliceit := iterator.Unslice(metait)
		if matcher == nil {
			iters = append(iters, unsliceit)
			continue
		}

		matchit := iterator.Filter(unsliceit, func(m metadata) bool {
			return matcher(m)
		})

		iters = append(iters, matchit)
	}

	return iterator.Aggregate(iters...), nil
}

func (p *systemFindFont) log(level log.Level, msg string, args ...any) {
	log.WithFields(log.Fields{
		logging.KeyClass: "font.Manager",
	}).Logf(level, msg, args...)
}

type matcher func(metadata) bool

func matchFamily(family string) matcher {
	return func(m metadata) bool {
		return strings.EqualFold(m.family, family)
	}
}

type metadata struct {
	family string
	path   string
}

func readMetadata(path string, r io.ReadSeeker) ([]metadata, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read font: %w", err)
	}
	col, err := sfnt.ParseCollection(data)
	if err != nil {
		return nil, fmt.Errorf("sfnt parse collection: %w", err)
	}
	if col.NumFonts() == 0 {
		return nil, errors.New("no fonts found in file")
	}
	ret := make([]metadata, col.NumFonts())
	for i := 0; i < col.NumFonts(); i++ {
		font, err := col.Font(i)
		if err != nil {
			return nil, fmt.Errorf("font %d of collection: %w", i, err)
		}
		var buf sfnt.Buffer
		family, err := font.Name(&buf, sfnt.NameIDFamily)
		if err != nil {
			return nil, fmt.Errorf("font %d read family: %w", i, err)
		}
		ret[i] = metadata{
			path:   path,
			family: family,
		}
	}
	return ret, nil
}

func expandUser(path string) (expandedPath string) {
	if strings.HasPrefix(path, "~") {
		if u, err := user.Current(); err == nil {
			return strings.Replace(path, "~", u.HomeDir, -1)
		}
	}
	return path
}
