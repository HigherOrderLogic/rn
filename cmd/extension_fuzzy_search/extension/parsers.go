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

package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/sirupsen/logrus"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"github.com/unstablebuild/blue/iterator"
	"unstable.build/go-tui/extension"
	"unstable.build/go-tui/ide/syntax"
)

type parser struct {
	closed bool
	parser *sitter.Parser
	lang   *sitter.Language
	query  *sitter.Query
	lib    uintptr
}

func newParser(
	ctx context.Context, langID string,
	pkg *extension.PkgManager,
	queryFile, query string,
) (ret *parser, err error) {
	it, err := pkg.LibDir(ctx, langID)
	if err != nil {
		err = fmt.Errorf("scan language package installation: %w", err)
		return
	}
	files, err := iterator.ToSlice(ctx, it)
	_ = it.Close()
	if err != nil {
		err = fmt.Errorf("list files: %w", err)
		return
	}
	var langfile string
	logrus.Tracef("found files: %v", files)
	for _, path := range files {
		switch filepath.Base(path) {
		case syntax.ParserFilename:
			langfile = path
		case queryFile:
			// override path with absolute path
			queryFile = path
		}
	}

	if langfile == "" || (queryFile == "" && query == "") {
		err = errors.New("could not find parser file or " +
			"definitions in language installation")
		return
	}

	ret = new(parser)
	ret.lib, err = purego.Dlopen(langfile, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		err = fmt.Errorf("dlopen %q: %w", langfile, err)
		return
	}

	parserID := fmt.Sprintf("tree_sitter_%s", langID)

	var lang func() uintptr
	sym, err := purego.Dlsym(ret.lib, parserID)
	if err != nil {
		_ = purego.Dlclose(ret.lib)
		err = fmt.Errorf("load symbol %q: %w", parserID, err)
		return
	}
	purego.RegisterFunc(&lang, sym)

	language := sitter.NewLanguage(unsafe.Pointer(lang()))
	parser := sitter.NewParser()
	err = parser.SetLanguage(language)
	if err != nil {
		_ = purego.Dlclose(ret.lib)
		parser.Close()
		err = fmt.Errorf("set parser language: %v", err)
		return
	}
	ret.lang = language
	ret.parser = parser

	if query != "" {
		var qerr *sitter.QueryError
		ret.query, qerr = sitter.NewQuery(ret.lang, query)
		if qerr != nil {
			err = qerr
		}
	} else {
		err = ret.initQueryFile(language, queryFile)
	}
	if err != nil {
		_ = purego.Dlclose(ret.lib)
		parser.Close()
		err = errInvalidQuery
		return
	}
	return
}

func (p *parser) initQueryFile(
	language *sitter.Language, queryFile string,
) error {
	data, err := os.ReadFile(queryFile)
	if err != nil {
		return fmt.Errorf("read locals file: %v", err)
	}
	locals, qerr := sitter.NewQuery(language, string(data))
	if qerr != nil {
		return fmt.Errorf("compile query: %v", qerr)
	}
	p.query = locals
	return nil
}

func (t *parser) Close() (ret error) {
	if t.closed {
		return nil
	}
	t.closed = true
	t.parser.Close()
	t.query.Close()
	ret = purego.Dlclose(t.lib)
	return
}
