#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
QUERIES=$DIR/queries.go

# truncate
cat /dev/null > $QUERIES

cat << EOF > $QUERIES 
package plugin

import _ "embed"

type languageQueries struct {
	folds string
	highlights string
	indents string
	injections string
	locals string
}

var languageQueriesByName = map[string]languageQueries{
EOF

for language in $(/bin/ls $DIR/nvim-treesitter/queries | xargs); do \
  echo "\"$language\": languageQueries{" >> $QUERIES; \

  FOLDS=$DIR/nvim-treesitter/queries/$language/folds.scm
  HIGHLIGHTS=$DIR/nvim-treesitter/queries/$language/highlights.scm
  INDENTS=$DIR/nvim-treesitter/queries/$language/indents.scm
  INJECTIONS=$DIR/nvim-treesitter/queries/$language/injections.scm
  LOCALS=$DIR/nvim-treesitter/queries/$language/locals.scm

  if [ -f "$FOLDS" ]; then
    echo "folds: ${language}Folds," >> $QUERIES; \
  fi

  if [ -f "$HIGHLIGHTS" ]; then
    echo "highlights: ${language}Highlights," >> $QUERIES; \
  fi

  if [ -f "$INDENTS" ]; then
    echo "indents: ${language}Idents," >> $QUERIES; \
  fi

  if [ -f "$INJECTIONS" ]; then
    echo "injections: ${language}Injections," >> $QUERIES; \
  fi

  if [ -f "$LOCALS" ]; then
    echo "locals: ${language}Locals," >> $QUERIES; \
  fi

  echo "}," >> $QUERIES;

done

echo "}" >> $QUERIES

# define embeds
for language in $(/bin/ls $DIR/nvim-treesitter/queries | xargs); do \
  pushd $DIR > /dev/null

  FOLDS=nvim-treesitter/queries/$language/folds.scm
  HIGHLIGHTS=nvim-treesitter/queries/$language/highlights.scm
  INDENTS=nvim-treesitter/queries/$language/indents.scm
  INJECTIONS=nvim-treesitter/queries/$language/injections.scm
  LOCALS=nvim-treesitter/queries/$language/locals.scm

  echo "// Language $language scm embeds" >> $QUERIES; \

  if [ -f "$FOLDS" ]; then
	echo "//go:embed $FOLDS" >> $QUERIES; \
    echo "var ${language}Folds string" >> $QUERIES; \
  fi

  if [ -f "$HIGHLIGHTS" ]; then
	echo "//go:embed $HIGHLIGHTS" >> $QUERIES; \
    echo "var ${language}Highlights string" >> $QUERIES; \
  fi

  if [ -f "$INDENTS" ]; then
	echo "//go:embed $INDENTS" >> $QUERIES; \
    echo "var ${language}Idents string" >> $QUERIES; \
  fi

  if [ -f "$INJECTIONS" ]; then
	echo "//go:embed $INJECTIONS" >> $QUERIES; \
    echo "var ${language}Injections string" >> $QUERIES; \
  fi

  if [ -f "$LOCALS" ]; then
	echo "//go:embed $LOCALS" >> $QUERIES; \
    echo "var ${language}Locals string" >> $QUERIES; \
  fi

  echo -e "\n\n" >> $QUERIES; \

  popd > /dev/null

done

gofmt -w $QUERIES
