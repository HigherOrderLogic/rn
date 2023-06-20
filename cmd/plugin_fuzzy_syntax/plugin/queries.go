package plugin

import _ "embed"

type languageQueries struct {
	folds      string
	highlights string
	indents    string
	injections string
	locals     string
}

var languageQueriesByName = map[string]languageQueries{
	"ada": languageQueries{
		folds:      adaFolds,
		highlights: adaHighlights,
		locals:     adaLocals,
	},
	"agda": languageQueries{
		folds:      agdaFolds,
		highlights: agdaHighlights,
	},
	"arduino": languageQueries{
		folds:      arduinoFolds,
		highlights: arduinoHighlights,
		indents:    arduinoIdents,
		injections: arduinoInjections,
		locals:     arduinoLocals,
	},
	"astro": languageQueries{
		folds:      astroFolds,
		highlights: astroHighlights,
		indents:    astroIdents,
		injections: astroInjections,
		locals:     astroLocals,
	},
	"awk": languageQueries{
		highlights: awkHighlights,
		injections: awkInjections,
	},
	"bash": languageQueries{
		folds:      bashFolds,
		highlights: bashHighlights,
		injections: bashInjections,
		locals:     bashLocals,
	},
	"bass": languageQueries{
		folds:      bassFolds,
		highlights: bassHighlights,
		indents:    bassIdents,
		injections: bassInjections,
		locals:     bassLocals,
	},
	"beancount": languageQueries{
		folds:      beancountFolds,
		highlights: beancountHighlights,
	},
	"bibtex": languageQueries{
		folds:      bibtexFolds,
		highlights: bibtexHighlights,
		indents:    bibtexIdents,
	},
	"bicep": languageQueries{
		folds:      bicepFolds,
		highlights: bicepHighlights,
		indents:    bicepIdents,
		injections: bicepInjections,
		locals:     bicepLocals,
	},
	"blueprint": languageQueries{
		highlights: blueprintHighlights,
	},
	"c": languageQueries{
		folds:      cFolds,
		highlights: cHighlights,
		indents:    cIdents,
		injections: cInjections,
		locals:     cLocals,
	},
	"c_sharp": languageQueries{
		folds:      c_sharpFolds,
		highlights: c_sharpHighlights,
		injections: c_sharpInjections,
		locals:     c_sharpLocals,
	},
	"capnp": languageQueries{
		folds:      capnpFolds,
		highlights: capnpHighlights,
		indents:    capnpIdents,
		injections: capnpInjections,
		locals:     capnpLocals,
	},
	"chatito": languageQueries{
		folds:      chatitoFolds,
		highlights: chatitoHighlights,
		indents:    chatitoIdents,
		injections: chatitoInjections,
		locals:     chatitoLocals,
	},
	"clojure": languageQueries{
		folds:      clojureFolds,
		highlights: clojureHighlights,
		injections: clojureInjections,
		locals:     clojureLocals,
	},
	"cmake": languageQueries{
		folds:      cmakeFolds,
		highlights: cmakeHighlights,
	},
	"comment": languageQueries{
		highlights: commentHighlights,
	},
	"commonlisp": languageQueries{
		folds:      commonlispFolds,
		highlights: commonlispHighlights,
		locals:     commonlispLocals,
	},
	"cooklang": languageQueries{
		highlights: cooklangHighlights,
	},
	"corn": languageQueries{
		folds:      cornFolds,
		highlights: cornHighlights,
		indents:    cornIdents,
		locals:     cornLocals,
	},
	"cpon": languageQueries{
		folds:      cponFolds,
		highlights: cponHighlights,
		indents:    cponIdents,
		injections: cponInjections,
		locals:     cponLocals,
	},
	"cpp": languageQueries{
		folds:      cppFolds,
		highlights: cppHighlights,
		indents:    cppIdents,
		injections: cppInjections,
		locals:     cppLocals,
	},
	"css": languageQueries{
		folds:      cssFolds,
		highlights: cssHighlights,
		indents:    cssIdents,
		injections: cssInjections,
	},
	"cuda": languageQueries{
		folds:      cudaFolds,
		highlights: cudaHighlights,
		indents:    cudaIdents,
		injections: cudaInjections,
		locals:     cudaLocals,
	},
	"cue": languageQueries{
		folds:      cueFolds,
		highlights: cueHighlights,
		indents:    cueIdents,
		injections: cueInjections,
		locals:     cueLocals,
	},
	"d": languageQueries{
		folds:      dFolds,
		highlights: dHighlights,
		indents:    dIdents,
		injections: dInjections,
	},
	"dart": languageQueries{
		folds:      dartFolds,
		highlights: dartHighlights,
		indents:    dartIdents,
		injections: dartInjections,
		locals:     dartLocals,
	},
	"devicetree": languageQueries{
		folds:      devicetreeFolds,
		highlights: devicetreeHighlights,
		indents:    devicetreeIdents,
		injections: devicetreeInjections,
		locals:     devicetreeLocals,
	},
	"dhall": languageQueries{
		folds:      dhallFolds,
		highlights: dhallHighlights,
		injections: dhallInjections,
	},
	"diff": languageQueries{
		highlights: diffHighlights,
	},
	"dockerfile": languageQueries{
		highlights: dockerfileHighlights,
		injections: dockerfileInjections,
	},
	"dot": languageQueries{
		highlights: dotHighlights,
		injections: dotInjections,
	},
	"ebnf": languageQueries{
		highlights: ebnfHighlights,
	},
	"ecma": languageQueries{
		folds:      ecmaFolds,
		highlights: ecmaHighlights,
		indents:    ecmaIdents,
		injections: ecmaInjections,
		locals:     ecmaLocals,
	},
	"eex": languageQueries{
		highlights: eexHighlights,
		injections: eexInjections,
	},
	"elixir": languageQueries{
		folds:      elixirFolds,
		highlights: elixirHighlights,
		indents:    elixirIdents,
		injections: elixirInjections,
		locals:     elixirLocals,
	},
	"elm": languageQueries{
		highlights: elmHighlights,
		injections: elmInjections,
	},
	"elsa": languageQueries{
		folds:      elsaFolds,
		highlights: elsaHighlights,
		indents:    elsaIdents,
		injections: elsaInjections,
		locals:     elsaLocals,
	},
	"elvish": languageQueries{
		highlights: elvishHighlights,
		injections: elvishInjections,
	},
	"embedded_template": languageQueries{
		highlights: embedded_templateHighlights,
		injections: embedded_templateInjections,
	},
	"erlang": languageQueries{
		folds:      erlangFolds,
		highlights: erlangHighlights,
	},
	"fennel": languageQueries{
		folds:      fennelFolds,
		highlights: fennelHighlights,
		injections: fennelInjections,
		locals:     fennelLocals,
	},
	"firrtl": languageQueries{
		folds:      firrtlFolds,
		highlights: firrtlHighlights,
		indents:    firrtlIdents,
		injections: firrtlInjections,
		locals:     firrtlLocals,
	},
	"fish": languageQueries{
		folds:      fishFolds,
		highlights: fishHighlights,
		indents:    fishIdents,
		injections: fishInjections,
		locals:     fishLocals,
	},
	"foam": languageQueries{
		folds:      foamFolds,
		highlights: foamHighlights,
		indents:    foamIdents,
		injections: foamInjections,
		locals:     foamLocals,
	},
	"fortran": languageQueries{
		folds:      fortranFolds,
		highlights: fortranHighlights,
		indents:    fortranIdents,
	},
	"fsh": languageQueries{
		highlights: fshHighlights,
	},
	"func": languageQueries{
		highlights: funcHighlights,
	},
	"fusion": languageQueries{
		folds:      fusionFolds,
		highlights: fusionHighlights,
		indents:    fusionIdents,
		locals:     fusionLocals,
	},
	"gdscript": languageQueries{
		folds:      gdscriptFolds,
		highlights: gdscriptHighlights,
		indents:    gdscriptIdents,
		injections: gdscriptInjections,
		locals:     gdscriptLocals,
	},
	"git_config": languageQueries{
		folds:      git_configFolds,
		highlights: git_configHighlights,
	},
	"git_rebase": languageQueries{
		highlights: git_rebaseHighlights,
		injections: git_rebaseInjections,
	},
	"gitattributes": languageQueries{
		highlights: gitattributesHighlights,
		injections: gitattributesInjections,
	},
	"gitcommit": languageQueries{
		highlights: gitcommitHighlights,
		injections: gitcommitInjections,
	},
	"gitignore": languageQueries{
		highlights: gitignoreHighlights,
	},
	"gleam": languageQueries{
		folds:      gleamFolds,
		highlights: gleamHighlights,
		indents:    gleamIdents,
		injections: gleamInjections,
		locals:     gleamLocals,
	},
	"glimmer": languageQueries{
		folds:      glimmerFolds,
		highlights: glimmerHighlights,
		indents:    glimmerIdents,
		locals:     glimmerLocals,
	},
	"glsl": languageQueries{
		folds:      glslFolds,
		highlights: glslHighlights,
		indents:    glslIdents,
		injections: glslInjections,
		locals:     glslLocals,
	},
	"go": languageQueries{
		folds:      goFolds,
		highlights: goHighlights,
		indents:    goIdents,
		injections: goInjections,
		locals:     goLocals,
	},
	"godot_resource": languageQueries{
		folds:      godot_resourceFolds,
		highlights: godot_resourceHighlights,
		locals:     godot_resourceLocals,
	},
	"gomod": languageQueries{
		highlights: gomodHighlights,
		injections: gomodInjections,
	},
	"gosum": languageQueries{
		highlights: gosumHighlights,
	},
	"gowork": languageQueries{
		highlights: goworkHighlights,
		injections: goworkInjections,
	},
	"graphql": languageQueries{
		highlights: graphqlHighlights,
		indents:    graphqlIdents,
		injections: graphqlInjections,
	},
	"hack": languageQueries{
		highlights: hackHighlights,
	},
	"hare": languageQueries{
		folds:      hareFolds,
		highlights: hareHighlights,
		indents:    hareIdents,
		injections: hareInjections,
		locals:     hareLocals,
	},
	"haskell": languageQueries{
		folds:      haskellFolds,
		highlights: haskellHighlights,
		injections: haskellInjections,
	},
	"hcl": languageQueries{
		folds:      hclFolds,
		highlights: hclHighlights,
		indents:    hclIdents,
		injections: hclInjections,
	},
	"heex": languageQueries{
		folds:      heexFolds,
		highlights: heexHighlights,
		indents:    heexIdents,
		injections: heexInjections,
		locals:     heexLocals,
	},
	"hjson": languageQueries{
		folds:      hjsonFolds,
		highlights: hjsonHighlights,
		indents:    hjsonIdents,
		injections: hjsonInjections,
		locals:     hjsonLocals,
	},
	"hlsl": languageQueries{
		folds:      hlslFolds,
		highlights: hlslHighlights,
		indents:    hlslIdents,
		injections: hlslInjections,
		locals:     hlslLocals,
	},
	"hocon": languageQueries{
		highlights: hoconHighlights,
		injections: hoconInjections,
	},
	"html": languageQueries{
		folds:      htmlFolds,
		highlights: htmlHighlights,
		indents:    htmlIdents,
		injections: htmlInjections,
		locals:     htmlLocals,
	},
	"html_tags": languageQueries{
		highlights: html_tagsHighlights,
		indents:    html_tagsIdents,
		injections: html_tagsInjections,
	},
	"htmldjango": languageQueries{
		folds:      htmldjangoFolds,
		highlights: htmldjangoHighlights,
		indents:    htmldjangoIdents,
		injections: htmldjangoInjections,
	},
	"http": languageQueries{
		highlights: httpHighlights,
		injections: httpInjections,
	},
	"ini": languageQueries{
		folds:      iniFolds,
		highlights: iniHighlights,
	},
	"ispc": languageQueries{
		folds:      ispcFolds,
		highlights: ispcHighlights,
		indents:    ispcIdents,
		injections: ispcInjections,
		locals:     ispcLocals,
	},
	"janet_simple": languageQueries{
		folds:      janet_simpleFolds,
		highlights: janet_simpleHighlights,
		injections: janet_simpleInjections,
		locals:     janet_simpleLocals,
	},
	"java": languageQueries{
		folds:      javaFolds,
		highlights: javaHighlights,
		indents:    javaIdents,
		injections: javaInjections,
		locals:     javaLocals,
	},
	"javascript": languageQueries{
		folds:      javascriptFolds,
		highlights: javascriptHighlights,
		indents:    javascriptIdents,
		injections: javascriptInjections,
		locals:     javascriptLocals,
	},
	"jq": languageQueries{
		highlights: jqHighlights,
		injections: jqInjections,
	},
	"jsdoc": languageQueries{
		highlights: jsdocHighlights,
	},
	"json": languageQueries{
		folds:      jsonFolds,
		highlights: jsonHighlights,
		indents:    jsonIdents,
		locals:     jsonLocals,
	},
	"json5": languageQueries{
		highlights: json5Highlights,
		injections: json5Injections,
	},
	"jsonc": languageQueries{
		folds:      jsoncFolds,
		highlights: jsoncHighlights,
		indents:    jsoncIdents,
		injections: jsoncInjections,
		locals:     jsoncLocals,
	},
	"jsonnet": languageQueries{
		highlights: jsonnetHighlights,
	},
	"jsx": languageQueries{
		folds:      jsxFolds,
		highlights: jsxHighlights,
		indents:    jsxIdents,
		injections: jsxInjections,
	},
	"julia": languageQueries{
		folds:      juliaFolds,
		highlights: juliaHighlights,
		indents:    juliaIdents,
		injections: juliaInjections,
		locals:     juliaLocals,
	},
	"kdl": languageQueries{
		folds:      kdlFolds,
		highlights: kdlHighlights,
		indents:    kdlIdents,
		injections: kdlInjections,
		locals:     kdlLocals,
	},
	"kotlin": languageQueries{
		folds:      kotlinFolds,
		highlights: kotlinHighlights,
		injections: kotlinInjections,
		locals:     kotlinLocals,
	},
	"lalrpop": languageQueries{
		highlights: lalrpopHighlights,
		injections: lalrpopInjections,
		locals:     lalrpopLocals,
	},
	"latex": languageQueries{
		folds:      latexFolds,
		highlights: latexHighlights,
		injections: latexInjections,
	},
	"ledger": languageQueries{
		folds:      ledgerFolds,
		highlights: ledgerHighlights,
		indents:    ledgerIdents,
		injections: ledgerInjections,
	},
	"llvm": languageQueries{
		highlights: llvmHighlights,
	},
	"lua": languageQueries{
		folds:      luaFolds,
		highlights: luaHighlights,
		indents:    luaIdents,
		injections: luaInjections,
		locals:     luaLocals,
	},
	"luadoc": languageQueries{
		highlights: luadocHighlights,
	},
	"luap": languageQueries{
		highlights: luapHighlights,
	},
	"luau": languageQueries{
		folds:      luauFolds,
		highlights: luauHighlights,
		indents:    luauIdents,
		injections: luauInjections,
		locals:     luauLocals,
	},
	"m68k": languageQueries{
		folds:      m68kFolds,
		highlights: m68kHighlights,
		injections: m68kInjections,
		locals:     m68kLocals,
	},
	"make": languageQueries{
		folds:      makeFolds,
		highlights: makeHighlights,
		injections: makeInjections,
	},
	"markdown": languageQueries{
		folds:      markdownFolds,
		highlights: markdownHighlights,
		indents:    markdownIdents,
		injections: markdownInjections,
	},
	"markdown_inline": languageQueries{
		highlights: markdown_inlineHighlights,
		injections: markdown_inlineInjections,
	},
	"matlab": languageQueries{
		folds:      matlabFolds,
		highlights: matlabHighlights,
		indents:    matlabIdents,
		injections: matlabInjections,
		locals:     matlabLocals,
	},
	"menhir": languageQueries{
		highlights: menhirHighlights,
		injections: menhirInjections,
	},
	"mermaid": languageQueries{
		highlights: mermaidHighlights,
	},
	"meson": languageQueries{
		folds:      mesonFolds,
		highlights: mesonHighlights,
		injections: mesonInjections,
	},
	"mlir": languageQueries{
		highlights: mlirHighlights,
		locals:     mlirLocals,
	},
	"nickel": languageQueries{
		highlights: nickelHighlights,
		indents:    nickelIdents,
	},
	"ninja": languageQueries{
		folds:      ninjaFolds,
		highlights: ninjaHighlights,
		indents:    ninjaIdents,
	},
	"nix": languageQueries{
		folds:      nixFolds,
		highlights: nixHighlights,
		injections: nixInjections,
		locals:     nixLocals,
	},
	"objc": languageQueries{
		folds:      objcFolds,
		highlights: objcHighlights,
		indents:    objcIdents,
		injections: objcInjections,
		locals:     objcLocals,
	},
	"ocaml": languageQueries{
		folds:      ocamlFolds,
		highlights: ocamlHighlights,
		indents:    ocamlIdents,
		injections: ocamlInjections,
		locals:     ocamlLocals,
	},
	"ocaml_interface": languageQueries{
		folds:      ocaml_interfaceFolds,
		highlights: ocaml_interfaceHighlights,
		indents:    ocaml_interfaceIdents,
		injections: ocaml_interfaceInjections,
		locals:     ocaml_interfaceLocals,
	},
	"ocamllex": languageQueries{
		highlights: ocamllexHighlights,
		injections: ocamllexInjections,
	},
	"odin": languageQueries{
		folds:      odinFolds,
		highlights: odinHighlights,
		indents:    odinIdents,
		injections: odinInjections,
		locals:     odinLocals,
	},
	"pascal": languageQueries{
		folds:      pascalFolds,
		highlights: pascalHighlights,
		indents:    pascalIdents,
		injections: pascalInjections,
		locals:     pascalLocals,
	},
	"passwd": languageQueries{
		highlights: passwdHighlights,
	},
	"perl": languageQueries{
		folds:      perlFolds,
		highlights: perlHighlights,
		injections: perlInjections,
	},
	"php": languageQueries{
		folds:      phpFolds,
		highlights: phpHighlights,
		indents:    phpIdents,
		injections: phpInjections,
		locals:     phpLocals,
	},
	"phpdoc": languageQueries{
		highlights: phpdocHighlights,
	},
	"pioasm": languageQueries{
		highlights: pioasmHighlights,
		injections: pioasmInjections,
	},
	"po": languageQueries{
		folds:      poFolds,
		highlights: poHighlights,
		injections: poInjections,
	},
	"poe_filter": languageQueries{
		folds:      poe_filterFolds,
		highlights: poe_filterHighlights,
		indents:    poe_filterIdents,
		injections: poe_filterInjections,
	},
	"pony": languageQueries{
		folds:      ponyFolds,
		highlights: ponyHighlights,
		indents:    ponyIdents,
		injections: ponyInjections,
		locals:     ponyLocals,
	},
	"prisma": languageQueries{
		highlights: prismaHighlights,
	},
	"proto": languageQueries{
		folds:      protoFolds,
		highlights: protoHighlights,
	},
	"prql": languageQueries{
		highlights: prqlHighlights,
		injections: prqlInjections,
	},
	"pug": languageQueries{
		highlights: pugHighlights,
		injections: pugInjections,
	},
	"puppet": languageQueries{
		folds:      puppetFolds,
		highlights: puppetHighlights,
		indents:    puppetIdents,
		injections: puppetInjections,
		locals:     puppetLocals,
	},
	"python": languageQueries{
		folds:      pythonFolds,
		highlights: pythonHighlights,
		indents:    pythonIdents,
		injections: pythonInjections,
		locals:     pythonLocals,
	},
	"ql": languageQueries{
		folds:      qlFolds,
		highlights: qlHighlights,
		indents:    qlIdents,
		injections: qlInjections,
		locals:     qlLocals,
	},
	"qmldir": languageQueries{
		highlights: qmldirHighlights,
		injections: qmldirInjections,
	},
	"qmljs": languageQueries{
		folds:      qmljsFolds,
		highlights: qmljsHighlights,
	},
	"query": languageQueries{
		folds:      queryFolds,
		highlights: queryHighlights,
		indents:    queryIdents,
		injections: queryInjections,
		locals:     queryLocals,
	},
	"r": languageQueries{
		highlights: rHighlights,
		indents:    rIdents,
		injections: rInjections,
		locals:     rLocals,
	},
	"racket": languageQueries{
		folds:      racketFolds,
		highlights: racketHighlights,
		injections: racketInjections,
	},
	"rasi": languageQueries{
		folds:      rasiFolds,
		highlights: rasiHighlights,
		indents:    rasiIdents,
		locals:     rasiLocals,
	},
	"regex": languageQueries{
		highlights: regexHighlights,
	},
	"rego": languageQueries{
		highlights: regoHighlights,
		injections: regoInjections,
	},
	"rnoweb": languageQueries{
		folds:      rnowebFolds,
		highlights: rnowebHighlights,
		injections: rnowebInjections,
	},
	"ron": languageQueries{
		folds:      ronFolds,
		highlights: ronHighlights,
		indents:    ronIdents,
		injections: ronInjections,
		locals:     ronLocals,
	},
	"rst": languageQueries{
		highlights: rstHighlights,
		injections: rstInjections,
		locals:     rstLocals,
	},
	"ruby": languageQueries{
		folds:      rubyFolds,
		highlights: rubyHighlights,
		indents:    rubyIdents,
		injections: rubyInjections,
		locals:     rubyLocals,
	},
	"rust": languageQueries{
		folds:      rustFolds,
		highlights: rustHighlights,
		indents:    rustIdents,
		injections: rustInjections,
		locals:     rustLocals,
	},
	"scala": languageQueries{
		folds:      scalaFolds,
		highlights: scalaHighlights,
		injections: scalaInjections,
		locals:     scalaLocals,
	},
	"scheme": languageQueries{
		folds:      schemeFolds,
		highlights: schemeHighlights,
		injections: schemeInjections,
	},
	"scss": languageQueries{
		folds:      scssFolds,
		highlights: scssHighlights,
		indents:    scssIdents,
	},
	"slint": languageQueries{
		highlights: slintHighlights,
		indents:    slintIdents,
	},
	"smali": languageQueries{
		folds:      smaliFolds,
		highlights: smaliHighlights,
		indents:    smaliIdents,
		injections: smaliInjections,
		locals:     smaliLocals,
	},
	"smithy": languageQueries{
		highlights: smithyHighlights,
	},
	"solidity": languageQueries{
		highlights: solidityHighlights,
	},
	"sparql": languageQueries{
		folds:      sparqlFolds,
		highlights: sparqlHighlights,
		indents:    sparqlIdents,
		injections: sparqlInjections,
		locals:     sparqlLocals,
	},
	"sql": languageQueries{
		highlights: sqlHighlights,
		indents:    sqlIdents,
		injections: sqlInjections,
	},
	"squirrel": languageQueries{
		folds:      squirrelFolds,
		highlights: squirrelHighlights,
		indents:    squirrelIdents,
		injections: squirrelInjections,
		locals:     squirrelLocals,
	},
	"starlark": languageQueries{
		folds:      starlarkFolds,
		highlights: starlarkHighlights,
		indents:    starlarkIdents,
		injections: starlarkInjections,
		locals:     starlarkLocals,
	},
	"supercollider": languageQueries{
		folds:      supercolliderFolds,
		highlights: supercolliderHighlights,
		indents:    supercolliderIdents,
		injections: supercolliderInjections,
		locals:     supercolliderLocals,
	},
	"surface": languageQueries{
		folds:      surfaceFolds,
		highlights: surfaceHighlights,
		indents:    surfaceIdents,
		injections: surfaceInjections,
	},
	"svelte": languageQueries{
		folds:      svelteFolds,
		highlights: svelteHighlights,
		indents:    svelteIdents,
		injections: svelteInjections,
	},
	"swift": languageQueries{
		highlights: swiftHighlights,
		indents:    swiftIdents,
		locals:     swiftLocals,
	},
	"sxhkdrc": languageQueries{
		folds:      sxhkdrcFolds,
		highlights: sxhkdrcHighlights,
		injections: sxhkdrcInjections,
	},
	"t32": languageQueries{
		folds:      t32Folds,
		highlights: t32Highlights,
		indents:    t32Idents,
		injections: t32Injections,
		locals:     t32Locals,
	},
	"tablegen": languageQueries{
		folds:      tablegenFolds,
		highlights: tablegenHighlights,
		indents:    tablegenIdents,
		injections: tablegenInjections,
		locals:     tablegenLocals,
	},
	"teal": languageQueries{
		folds:      tealFolds,
		highlights: tealHighlights,
		indents:    tealIdents,
		injections: tealInjections,
		locals:     tealLocals,
	},
	"terraform": languageQueries{
		folds:      terraformFolds,
		highlights: terraformHighlights,
		indents:    terraformIdents,
		injections: terraformInjections,
	},
	"thrift": languageQueries{
		folds:      thriftFolds,
		highlights: thriftHighlights,
		indents:    thriftIdents,
		injections: thriftInjections,
		locals:     thriftLocals,
	},
	"tiger": languageQueries{
		folds:      tigerFolds,
		highlights: tigerHighlights,
		indents:    tigerIdents,
		injections: tigerInjections,
		locals:     tigerLocals,
	},
	"tlaplus": languageQueries{
		folds:      tlaplusFolds,
		highlights: tlaplusHighlights,
		injections: tlaplusInjections,
		locals:     tlaplusLocals,
	},
	"todotxt": languageQueries{
		highlights: todotxtHighlights,
	},
	"toml": languageQueries{
		folds:      tomlFolds,
		highlights: tomlHighlights,
		indents:    tomlIdents,
		injections: tomlInjections,
		locals:     tomlLocals,
	},
	"tsx": languageQueries{
		folds:      tsxFolds,
		highlights: tsxHighlights,
		indents:    tsxIdents,
		injections: tsxInjections,
		locals:     tsxLocals,
	},
	"turtle": languageQueries{
		folds:      turtleFolds,
		highlights: turtleHighlights,
		indents:    turtleIdents,
		injections: turtleInjections,
		locals:     turtleLocals,
	},
	"twig": languageQueries{
		highlights: twigHighlights,
		injections: twigInjections,
	},
	"typescript": languageQueries{
		folds:      typescriptFolds,
		highlights: typescriptHighlights,
		indents:    typescriptIdents,
		injections: typescriptInjections,
		locals:     typescriptLocals,
	},
	"ungrammar": languageQueries{
		folds:      ungrammarFolds,
		highlights: ungrammarHighlights,
		indents:    ungrammarIdents,
		injections: ungrammarInjections,
		locals:     ungrammarLocals,
	},
	"usd": languageQueries{
		folds:      usdFolds,
		highlights: usdHighlights,
		indents:    usdIdents,
		locals:     usdLocals,
	},
	"uxntal": languageQueries{
		folds:      uxntalFolds,
		highlights: uxntalHighlights,
		indents:    uxntalIdents,
		injections: uxntalInjections,
		locals:     uxntalLocals,
	},
	"v": languageQueries{
		folds:      vFolds,
		highlights: vHighlights,
		indents:    vIdents,
		injections: vInjections,
		locals:     vLocals,
	},
	"vala": languageQueries{
		folds:      valaFolds,
		highlights: valaHighlights,
	},
	"verilog": languageQueries{
		folds:      verilogFolds,
		highlights: verilogHighlights,
		injections: verilogInjections,
		locals:     verilogLocals,
	},
	"vhs": languageQueries{
		highlights: vhsHighlights,
	},
	"vim": languageQueries{
		folds:      vimFolds,
		highlights: vimHighlights,
		injections: vimInjections,
		locals:     vimLocals,
	},
	"vimdoc": languageQueries{
		highlights: vimdocHighlights,
		injections: vimdocInjections,
	},
	"vue": languageQueries{
		folds:      vueFolds,
		highlights: vueHighlights,
		indents:    vueIdents,
		injections: vueInjections,
	},
	"wgsl": languageQueries{
		folds:      wgslFolds,
		highlights: wgslHighlights,
		indents:    wgslIdents,
	},
	"wgsl_bevy": languageQueries{
		folds:      wgsl_bevyFolds,
		highlights: wgsl_bevyHighlights,
		indents:    wgsl_bevyIdents,
	},
	"yaml": languageQueries{
		folds:      yamlFolds,
		highlights: yamlHighlights,
		indents:    yamlIdents,
		injections: yamlInjections,
		locals:     yamlLocals,
	},
	"yang": languageQueries{
		folds:      yangFolds,
		highlights: yangHighlights,
		indents:    yangIdents,
		injections: yangInjections,
	},
	"yuck": languageQueries{
		folds:      yuckFolds,
		highlights: yuckHighlights,
		indents:    yuckIdents,
		injections: yuckInjections,
		locals:     yuckLocals,
	},
	"zig": languageQueries{
		folds:      zigFolds,
		highlights: zigHighlights,
		indents:    zigIdents,
		injections: zigInjections,
		locals:     zigLocals,
	},
}

// Language ada scm embeds
//
//go:embed nvim-treesitter/queries/ada/folds.scm
var adaFolds string

//go:embed nvim-treesitter/queries/ada/highlights.scm
var adaHighlights string

//go:embed nvim-treesitter/queries/ada/locals.scm
var adaLocals string

// Language agda scm embeds
//
//go:embed nvim-treesitter/queries/agda/folds.scm
var agdaFolds string

//go:embed nvim-treesitter/queries/agda/highlights.scm
var agdaHighlights string

// Language arduino scm embeds
//
//go:embed nvim-treesitter/queries/arduino/folds.scm
var arduinoFolds string

//go:embed nvim-treesitter/queries/arduino/highlights.scm
var arduinoHighlights string

//go:embed nvim-treesitter/queries/arduino/indents.scm
var arduinoIdents string

//go:embed nvim-treesitter/queries/arduino/injections.scm
var arduinoInjections string

//go:embed nvim-treesitter/queries/arduino/locals.scm
var arduinoLocals string

// Language astro scm embeds
//
//go:embed nvim-treesitter/queries/astro/folds.scm
var astroFolds string

//go:embed nvim-treesitter/queries/astro/highlights.scm
var astroHighlights string

//go:embed nvim-treesitter/queries/astro/indents.scm
var astroIdents string

//go:embed nvim-treesitter/queries/astro/injections.scm
var astroInjections string

//go:embed nvim-treesitter/queries/astro/locals.scm
var astroLocals string

// Language awk scm embeds
//
//go:embed nvim-treesitter/queries/awk/highlights.scm
var awkHighlights string

//go:embed nvim-treesitter/queries/awk/injections.scm
var awkInjections string

// Language bash scm embeds
//
//go:embed nvim-treesitter/queries/bash/folds.scm
var bashFolds string

//go:embed nvim-treesitter/queries/bash/highlights.scm
var bashHighlights string

//go:embed nvim-treesitter/queries/bash/injections.scm
var bashInjections string

//go:embed nvim-treesitter/queries/bash/locals.scm
var bashLocals string

// Language bass scm embeds
//
//go:embed nvim-treesitter/queries/bass/folds.scm
var bassFolds string

//go:embed nvim-treesitter/queries/bass/highlights.scm
var bassHighlights string

//go:embed nvim-treesitter/queries/bass/indents.scm
var bassIdents string

//go:embed nvim-treesitter/queries/bass/injections.scm
var bassInjections string

//go:embed nvim-treesitter/queries/bass/locals.scm
var bassLocals string

// Language beancount scm embeds
//
//go:embed nvim-treesitter/queries/beancount/folds.scm
var beancountFolds string

//go:embed nvim-treesitter/queries/beancount/highlights.scm
var beancountHighlights string

// Language bibtex scm embeds
//
//go:embed nvim-treesitter/queries/bibtex/folds.scm
var bibtexFolds string

//go:embed nvim-treesitter/queries/bibtex/highlights.scm
var bibtexHighlights string

//go:embed nvim-treesitter/queries/bibtex/indents.scm
var bibtexIdents string

// Language bicep scm embeds
//
//go:embed nvim-treesitter/queries/bicep/folds.scm
var bicepFolds string

//go:embed nvim-treesitter/queries/bicep/highlights.scm
var bicepHighlights string

//go:embed nvim-treesitter/queries/bicep/indents.scm
var bicepIdents string

//go:embed nvim-treesitter/queries/bicep/injections.scm
var bicepInjections string

//go:embed nvim-treesitter/queries/bicep/locals.scm
var bicepLocals string

// Language blueprint scm embeds
//
//go:embed nvim-treesitter/queries/blueprint/highlights.scm
var blueprintHighlights string

// Language c scm embeds
//
//go:embed nvim-treesitter/queries/c/folds.scm
var cFolds string

//go:embed nvim-treesitter/queries/c/highlights.scm
var cHighlights string

//go:embed nvim-treesitter/queries/c/indents.scm
var cIdents string

//go:embed nvim-treesitter/queries/c/injections.scm
var cInjections string

//go:embed nvim-treesitter/queries/c/locals.scm
var cLocals string

// Language c_sharp scm embeds
//
//go:embed nvim-treesitter/queries/c_sharp/folds.scm
var c_sharpFolds string

//go:embed nvim-treesitter/queries/c_sharp/highlights.scm
var c_sharpHighlights string

//go:embed nvim-treesitter/queries/c_sharp/injections.scm
var c_sharpInjections string

//go:embed nvim-treesitter/queries/c_sharp/locals.scm
var c_sharpLocals string

// Language capnp scm embeds
//
//go:embed nvim-treesitter/queries/capnp/folds.scm
var capnpFolds string

//go:embed nvim-treesitter/queries/capnp/highlights.scm
var capnpHighlights string

//go:embed nvim-treesitter/queries/capnp/indents.scm
var capnpIdents string

//go:embed nvim-treesitter/queries/capnp/injections.scm
var capnpInjections string

//go:embed nvim-treesitter/queries/capnp/locals.scm
var capnpLocals string

// Language chatito scm embeds
//
//go:embed nvim-treesitter/queries/chatito/folds.scm
var chatitoFolds string

//go:embed nvim-treesitter/queries/chatito/highlights.scm
var chatitoHighlights string

//go:embed nvim-treesitter/queries/chatito/indents.scm
var chatitoIdents string

//go:embed nvim-treesitter/queries/chatito/injections.scm
var chatitoInjections string

//go:embed nvim-treesitter/queries/chatito/locals.scm
var chatitoLocals string

// Language clojure scm embeds
//
//go:embed nvim-treesitter/queries/clojure/folds.scm
var clojureFolds string

//go:embed nvim-treesitter/queries/clojure/highlights.scm
var clojureHighlights string

//go:embed nvim-treesitter/queries/clojure/injections.scm
var clojureInjections string

//go:embed nvim-treesitter/queries/clojure/locals.scm
var clojureLocals string

// Language cmake scm embeds
//
//go:embed nvim-treesitter/queries/cmake/folds.scm
var cmakeFolds string

//go:embed nvim-treesitter/queries/cmake/highlights.scm
var cmakeHighlights string

// Language comment scm embeds
//
//go:embed nvim-treesitter/queries/comment/highlights.scm
var commentHighlights string

// Language commonlisp scm embeds
//
//go:embed nvim-treesitter/queries/commonlisp/folds.scm
var commonlispFolds string

//go:embed nvim-treesitter/queries/commonlisp/highlights.scm
var commonlispHighlights string

//go:embed nvim-treesitter/queries/commonlisp/locals.scm
var commonlispLocals string

// Language cooklang scm embeds
//
//go:embed nvim-treesitter/queries/cooklang/highlights.scm
var cooklangHighlights string

// Language corn scm embeds
//
//go:embed nvim-treesitter/queries/corn/folds.scm
var cornFolds string

//go:embed nvim-treesitter/queries/corn/highlights.scm
var cornHighlights string

//go:embed nvim-treesitter/queries/corn/indents.scm
var cornIdents string

//go:embed nvim-treesitter/queries/corn/locals.scm
var cornLocals string

// Language cpon scm embeds
//
//go:embed nvim-treesitter/queries/cpon/folds.scm
var cponFolds string

//go:embed nvim-treesitter/queries/cpon/highlights.scm
var cponHighlights string

//go:embed nvim-treesitter/queries/cpon/indents.scm
var cponIdents string

//go:embed nvim-treesitter/queries/cpon/injections.scm
var cponInjections string

//go:embed nvim-treesitter/queries/cpon/locals.scm
var cponLocals string

// Language cpp scm embeds
//
//go:embed nvim-treesitter/queries/cpp/folds.scm
var cppFolds string

//go:embed nvim-treesitter/queries/cpp/highlights.scm
var cppHighlights string

//go:embed nvim-treesitter/queries/cpp/indents.scm
var cppIdents string

//go:embed nvim-treesitter/queries/cpp/injections.scm
var cppInjections string

//go:embed nvim-treesitter/queries/cpp/locals.scm
var cppLocals string

// Language css scm embeds
//
//go:embed nvim-treesitter/queries/css/folds.scm
var cssFolds string

//go:embed nvim-treesitter/queries/css/highlights.scm
var cssHighlights string

//go:embed nvim-treesitter/queries/css/indents.scm
var cssIdents string

//go:embed nvim-treesitter/queries/css/injections.scm
var cssInjections string

// Language cuda scm embeds
//
//go:embed nvim-treesitter/queries/cuda/folds.scm
var cudaFolds string

//go:embed nvim-treesitter/queries/cuda/highlights.scm
var cudaHighlights string

//go:embed nvim-treesitter/queries/cuda/indents.scm
var cudaIdents string

//go:embed nvim-treesitter/queries/cuda/injections.scm
var cudaInjections string

//go:embed nvim-treesitter/queries/cuda/locals.scm
var cudaLocals string

// Language cue scm embeds
//
//go:embed nvim-treesitter/queries/cue/folds.scm
var cueFolds string

//go:embed nvim-treesitter/queries/cue/highlights.scm
var cueHighlights string

//go:embed nvim-treesitter/queries/cue/indents.scm
var cueIdents string

//go:embed nvim-treesitter/queries/cue/injections.scm
var cueInjections string

//go:embed nvim-treesitter/queries/cue/locals.scm
var cueLocals string

// Language d scm embeds
//
//go:embed nvim-treesitter/queries/d/folds.scm
var dFolds string

//go:embed nvim-treesitter/queries/d/highlights.scm
var dHighlights string

//go:embed nvim-treesitter/queries/d/indents.scm
var dIdents string

//go:embed nvim-treesitter/queries/d/injections.scm
var dInjections string

// Language dart scm embeds
//
//go:embed nvim-treesitter/queries/dart/folds.scm
var dartFolds string

//go:embed nvim-treesitter/queries/dart/highlights.scm
var dartHighlights string

//go:embed nvim-treesitter/queries/dart/indents.scm
var dartIdents string

//go:embed nvim-treesitter/queries/dart/injections.scm
var dartInjections string

//go:embed nvim-treesitter/queries/dart/locals.scm
var dartLocals string

// Language devicetree scm embeds
//
//go:embed nvim-treesitter/queries/devicetree/folds.scm
var devicetreeFolds string

//go:embed nvim-treesitter/queries/devicetree/highlights.scm
var devicetreeHighlights string

//go:embed nvim-treesitter/queries/devicetree/indents.scm
var devicetreeIdents string

//go:embed nvim-treesitter/queries/devicetree/injections.scm
var devicetreeInjections string

//go:embed nvim-treesitter/queries/devicetree/locals.scm
var devicetreeLocals string

// Language dhall scm embeds
//
//go:embed nvim-treesitter/queries/dhall/folds.scm
var dhallFolds string

//go:embed nvim-treesitter/queries/dhall/highlights.scm
var dhallHighlights string

//go:embed nvim-treesitter/queries/dhall/injections.scm
var dhallInjections string

// Language diff scm embeds
//
//go:embed nvim-treesitter/queries/diff/highlights.scm
var diffHighlights string

// Language dockerfile scm embeds
//
//go:embed nvim-treesitter/queries/dockerfile/highlights.scm
var dockerfileHighlights string

//go:embed nvim-treesitter/queries/dockerfile/injections.scm
var dockerfileInjections string

// Language dot scm embeds
//
//go:embed nvim-treesitter/queries/dot/highlights.scm
var dotHighlights string

//go:embed nvim-treesitter/queries/dot/injections.scm
var dotInjections string

// Language ebnf scm embeds
//
//go:embed nvim-treesitter/queries/ebnf/highlights.scm
var ebnfHighlights string

// Language ecma scm embeds
//
//go:embed nvim-treesitter/queries/ecma/folds.scm
var ecmaFolds string

//go:embed nvim-treesitter/queries/ecma/highlights.scm
var ecmaHighlights string

//go:embed nvim-treesitter/queries/ecma/indents.scm
var ecmaIdents string

//go:embed nvim-treesitter/queries/ecma/injections.scm
var ecmaInjections string

//go:embed nvim-treesitter/queries/ecma/locals.scm
var ecmaLocals string

// Language eex scm embeds
//
//go:embed nvim-treesitter/queries/eex/highlights.scm
var eexHighlights string

//go:embed nvim-treesitter/queries/eex/injections.scm
var eexInjections string

// Language elixir scm embeds
//
//go:embed nvim-treesitter/queries/elixir/folds.scm
var elixirFolds string

//go:embed nvim-treesitter/queries/elixir/highlights.scm
var elixirHighlights string

//go:embed nvim-treesitter/queries/elixir/indents.scm
var elixirIdents string

//go:embed nvim-treesitter/queries/elixir/injections.scm
var elixirInjections string

//go:embed nvim-treesitter/queries/elixir/locals.scm
var elixirLocals string

// Language elm scm embeds
//
//go:embed nvim-treesitter/queries/elm/highlights.scm
var elmHighlights string

//go:embed nvim-treesitter/queries/elm/injections.scm
var elmInjections string

// Language elsa scm embeds
//
//go:embed nvim-treesitter/queries/elsa/folds.scm
var elsaFolds string

//go:embed nvim-treesitter/queries/elsa/highlights.scm
var elsaHighlights string

//go:embed nvim-treesitter/queries/elsa/indents.scm
var elsaIdents string

//go:embed nvim-treesitter/queries/elsa/injections.scm
var elsaInjections string

//go:embed nvim-treesitter/queries/elsa/locals.scm
var elsaLocals string

// Language elvish scm embeds
//
//go:embed nvim-treesitter/queries/elvish/highlights.scm
var elvishHighlights string

//go:embed nvim-treesitter/queries/elvish/injections.scm
var elvishInjections string

// Language embedded_template scm embeds
//
//go:embed nvim-treesitter/queries/embedded_template/highlights.scm
var embedded_templateHighlights string

//go:embed nvim-treesitter/queries/embedded_template/injections.scm
var embedded_templateInjections string

// Language erlang scm embeds
//
//go:embed nvim-treesitter/queries/erlang/folds.scm
var erlangFolds string

//go:embed nvim-treesitter/queries/erlang/highlights.scm
var erlangHighlights string

// Language fennel scm embeds
//
//go:embed nvim-treesitter/queries/fennel/folds.scm
var fennelFolds string

//go:embed nvim-treesitter/queries/fennel/highlights.scm
var fennelHighlights string

//go:embed nvim-treesitter/queries/fennel/injections.scm
var fennelInjections string

//go:embed nvim-treesitter/queries/fennel/locals.scm
var fennelLocals string

// Language firrtl scm embeds
//
//go:embed nvim-treesitter/queries/firrtl/folds.scm
var firrtlFolds string

//go:embed nvim-treesitter/queries/firrtl/highlights.scm
var firrtlHighlights string

//go:embed nvim-treesitter/queries/firrtl/indents.scm
var firrtlIdents string

//go:embed nvim-treesitter/queries/firrtl/injections.scm
var firrtlInjections string

//go:embed nvim-treesitter/queries/firrtl/locals.scm
var firrtlLocals string

// Language fish scm embeds
//
//go:embed nvim-treesitter/queries/fish/folds.scm
var fishFolds string

//go:embed nvim-treesitter/queries/fish/highlights.scm
var fishHighlights string

//go:embed nvim-treesitter/queries/fish/indents.scm
var fishIdents string

//go:embed nvim-treesitter/queries/fish/injections.scm
var fishInjections string

//go:embed nvim-treesitter/queries/fish/locals.scm
var fishLocals string

// Language foam scm embeds
//
//go:embed nvim-treesitter/queries/foam/folds.scm
var foamFolds string

//go:embed nvim-treesitter/queries/foam/highlights.scm
var foamHighlights string

//go:embed nvim-treesitter/queries/foam/indents.scm
var foamIdents string

//go:embed nvim-treesitter/queries/foam/injections.scm
var foamInjections string

//go:embed nvim-treesitter/queries/foam/locals.scm
var foamLocals string

// Language fortran scm embeds
//
//go:embed nvim-treesitter/queries/fortran/folds.scm
var fortranFolds string

//go:embed nvim-treesitter/queries/fortran/highlights.scm
var fortranHighlights string

//go:embed nvim-treesitter/queries/fortran/indents.scm
var fortranIdents string

// Language fsh scm embeds
//
//go:embed nvim-treesitter/queries/fsh/highlights.scm
var fshHighlights string

// Language func scm embeds
//
//go:embed nvim-treesitter/queries/func/highlights.scm
var funcHighlights string

// Language fusion scm embeds
//
//go:embed nvim-treesitter/queries/fusion/folds.scm
var fusionFolds string

//go:embed nvim-treesitter/queries/fusion/highlights.scm
var fusionHighlights string

//go:embed nvim-treesitter/queries/fusion/indents.scm
var fusionIdents string

//go:embed nvim-treesitter/queries/fusion/locals.scm
var fusionLocals string

// Language gdscript scm embeds
//
//go:embed nvim-treesitter/queries/gdscript/folds.scm
var gdscriptFolds string

//go:embed nvim-treesitter/queries/gdscript/highlights.scm
var gdscriptHighlights string

//go:embed nvim-treesitter/queries/gdscript/indents.scm
var gdscriptIdents string

//go:embed nvim-treesitter/queries/gdscript/injections.scm
var gdscriptInjections string

//go:embed nvim-treesitter/queries/gdscript/locals.scm
var gdscriptLocals string

// Language git_config scm embeds
//
//go:embed nvim-treesitter/queries/git_config/folds.scm
var git_configFolds string

//go:embed nvim-treesitter/queries/git_config/highlights.scm
var git_configHighlights string

// Language git_rebase scm embeds
//
//go:embed nvim-treesitter/queries/git_rebase/highlights.scm
var git_rebaseHighlights string

//go:embed nvim-treesitter/queries/git_rebase/injections.scm
var git_rebaseInjections string

// Language gitattributes scm embeds
//
//go:embed nvim-treesitter/queries/gitattributes/highlights.scm
var gitattributesHighlights string

//go:embed nvim-treesitter/queries/gitattributes/injections.scm
var gitattributesInjections string

// Language gitcommit scm embeds
//
//go:embed nvim-treesitter/queries/gitcommit/highlights.scm
var gitcommitHighlights string

//go:embed nvim-treesitter/queries/gitcommit/injections.scm
var gitcommitInjections string

// Language gitignore scm embeds
//
//go:embed nvim-treesitter/queries/gitignore/highlights.scm
var gitignoreHighlights string

// Language gleam scm embeds
//
//go:embed nvim-treesitter/queries/gleam/folds.scm
var gleamFolds string

//go:embed nvim-treesitter/queries/gleam/highlights.scm
var gleamHighlights string

//go:embed nvim-treesitter/queries/gleam/indents.scm
var gleamIdents string

//go:embed nvim-treesitter/queries/gleam/injections.scm
var gleamInjections string

//go:embed nvim-treesitter/queries/gleam/locals.scm
var gleamLocals string

// Language glimmer scm embeds
//
//go:embed nvim-treesitter/queries/glimmer/folds.scm
var glimmerFolds string

//go:embed nvim-treesitter/queries/glimmer/highlights.scm
var glimmerHighlights string

//go:embed nvim-treesitter/queries/glimmer/indents.scm
var glimmerIdents string

//go:embed nvim-treesitter/queries/glimmer/locals.scm
var glimmerLocals string

// Language glsl scm embeds
//
//go:embed nvim-treesitter/queries/glsl/folds.scm
var glslFolds string

//go:embed nvim-treesitter/queries/glsl/highlights.scm
var glslHighlights string

//go:embed nvim-treesitter/queries/glsl/indents.scm
var glslIdents string

//go:embed nvim-treesitter/queries/glsl/injections.scm
var glslInjections string

//go:embed nvim-treesitter/queries/glsl/locals.scm
var glslLocals string

// Language go scm embeds
//
//go:embed nvim-treesitter/queries/go/folds.scm
var goFolds string

//go:embed nvim-treesitter/queries/go/highlights.scm
var goHighlights string

//go:embed nvim-treesitter/queries/go/indents.scm
var goIdents string

//go:embed nvim-treesitter/queries/go/injections.scm
var goInjections string

//go:embed nvim-treesitter/queries/go/locals.scm
var goLocals string

// Language godot_resource scm embeds
//
//go:embed nvim-treesitter/queries/godot_resource/folds.scm
var godot_resourceFolds string

//go:embed nvim-treesitter/queries/godot_resource/highlights.scm
var godot_resourceHighlights string

//go:embed nvim-treesitter/queries/godot_resource/locals.scm
var godot_resourceLocals string

// Language gomod scm embeds
//
//go:embed nvim-treesitter/queries/gomod/highlights.scm
var gomodHighlights string

//go:embed nvim-treesitter/queries/gomod/injections.scm
var gomodInjections string

// Language gosum scm embeds
//
//go:embed nvim-treesitter/queries/gosum/highlights.scm
var gosumHighlights string

// Language gowork scm embeds
//
//go:embed nvim-treesitter/queries/gowork/highlights.scm
var goworkHighlights string

//go:embed nvim-treesitter/queries/gowork/injections.scm
var goworkInjections string

// Language graphql scm embeds
//
//go:embed nvim-treesitter/queries/graphql/highlights.scm
var graphqlHighlights string

//go:embed nvim-treesitter/queries/graphql/indents.scm
var graphqlIdents string

//go:embed nvim-treesitter/queries/graphql/injections.scm
var graphqlInjections string

// Language hack scm embeds
//
//go:embed nvim-treesitter/queries/hack/highlights.scm
var hackHighlights string

// Language hare scm embeds
//
//go:embed nvim-treesitter/queries/hare/folds.scm
var hareFolds string

//go:embed nvim-treesitter/queries/hare/highlights.scm
var hareHighlights string

//go:embed nvim-treesitter/queries/hare/indents.scm
var hareIdents string

//go:embed nvim-treesitter/queries/hare/injections.scm
var hareInjections string

//go:embed nvim-treesitter/queries/hare/locals.scm
var hareLocals string

// Language haskell scm embeds
//
//go:embed nvim-treesitter/queries/haskell/folds.scm
var haskellFolds string

//go:embed nvim-treesitter/queries/haskell/highlights.scm
var haskellHighlights string

//go:embed nvim-treesitter/queries/haskell/injections.scm
var haskellInjections string

// Language hcl scm embeds
//
//go:embed nvim-treesitter/queries/hcl/folds.scm
var hclFolds string

//go:embed nvim-treesitter/queries/hcl/highlights.scm
var hclHighlights string

//go:embed nvim-treesitter/queries/hcl/indents.scm
var hclIdents string

//go:embed nvim-treesitter/queries/hcl/injections.scm
var hclInjections string

// Language heex scm embeds
//
//go:embed nvim-treesitter/queries/heex/folds.scm
var heexFolds string

//go:embed nvim-treesitter/queries/heex/highlights.scm
var heexHighlights string

//go:embed nvim-treesitter/queries/heex/indents.scm
var heexIdents string

//go:embed nvim-treesitter/queries/heex/injections.scm
var heexInjections string

//go:embed nvim-treesitter/queries/heex/locals.scm
var heexLocals string

// Language hjson scm embeds
//
//go:embed nvim-treesitter/queries/hjson/folds.scm
var hjsonFolds string

//go:embed nvim-treesitter/queries/hjson/highlights.scm
var hjsonHighlights string

//go:embed nvim-treesitter/queries/hjson/indents.scm
var hjsonIdents string

//go:embed nvim-treesitter/queries/hjson/injections.scm
var hjsonInjections string

//go:embed nvim-treesitter/queries/hjson/locals.scm
var hjsonLocals string

// Language hlsl scm embeds
//
//go:embed nvim-treesitter/queries/hlsl/folds.scm
var hlslFolds string

//go:embed nvim-treesitter/queries/hlsl/highlights.scm
var hlslHighlights string

//go:embed nvim-treesitter/queries/hlsl/indents.scm
var hlslIdents string

//go:embed nvim-treesitter/queries/hlsl/injections.scm
var hlslInjections string

//go:embed nvim-treesitter/queries/hlsl/locals.scm
var hlslLocals string

// Language hocon scm embeds
//
//go:embed nvim-treesitter/queries/hocon/highlights.scm
var hoconHighlights string

//go:embed nvim-treesitter/queries/hocon/injections.scm
var hoconInjections string

// Language html scm embeds
//
//go:embed nvim-treesitter/queries/html/folds.scm
var htmlFolds string

//go:embed nvim-treesitter/queries/html/highlights.scm
var htmlHighlights string

//go:embed nvim-treesitter/queries/html/indents.scm
var htmlIdents string

//go:embed nvim-treesitter/queries/html/injections.scm
var htmlInjections string

//go:embed nvim-treesitter/queries/html/locals.scm
var htmlLocals string

// Language html_tags scm embeds
//
//go:embed nvim-treesitter/queries/html_tags/highlights.scm
var html_tagsHighlights string

//go:embed nvim-treesitter/queries/html_tags/indents.scm
var html_tagsIdents string

//go:embed nvim-treesitter/queries/html_tags/injections.scm
var html_tagsInjections string

// Language htmldjango scm embeds
//
//go:embed nvim-treesitter/queries/htmldjango/folds.scm
var htmldjangoFolds string

//go:embed nvim-treesitter/queries/htmldjango/highlights.scm
var htmldjangoHighlights string

//go:embed nvim-treesitter/queries/htmldjango/indents.scm
var htmldjangoIdents string

//go:embed nvim-treesitter/queries/htmldjango/injections.scm
var htmldjangoInjections string

// Language http scm embeds
//
//go:embed nvim-treesitter/queries/http/highlights.scm
var httpHighlights string

//go:embed nvim-treesitter/queries/http/injections.scm
var httpInjections string

// Language ini scm embeds
//
//go:embed nvim-treesitter/queries/ini/folds.scm
var iniFolds string

//go:embed nvim-treesitter/queries/ini/highlights.scm
var iniHighlights string

// Language ispc scm embeds
//
//go:embed nvim-treesitter/queries/ispc/folds.scm
var ispcFolds string

//go:embed nvim-treesitter/queries/ispc/highlights.scm
var ispcHighlights string

//go:embed nvim-treesitter/queries/ispc/indents.scm
var ispcIdents string

//go:embed nvim-treesitter/queries/ispc/injections.scm
var ispcInjections string

//go:embed nvim-treesitter/queries/ispc/locals.scm
var ispcLocals string

// Language janet_simple scm embeds
//
//go:embed nvim-treesitter/queries/janet_simple/folds.scm
var janet_simpleFolds string

//go:embed nvim-treesitter/queries/janet_simple/highlights.scm
var janet_simpleHighlights string

//go:embed nvim-treesitter/queries/janet_simple/injections.scm
var janet_simpleInjections string

//go:embed nvim-treesitter/queries/janet_simple/locals.scm
var janet_simpleLocals string

// Language java scm embeds
//
//go:embed nvim-treesitter/queries/java/folds.scm
var javaFolds string

//go:embed nvim-treesitter/queries/java/highlights.scm
var javaHighlights string

//go:embed nvim-treesitter/queries/java/indents.scm
var javaIdents string

//go:embed nvim-treesitter/queries/java/injections.scm
var javaInjections string

//go:embed nvim-treesitter/queries/java/locals.scm
var javaLocals string

// Language javascript scm embeds
//
//go:embed nvim-treesitter/queries/javascript/folds.scm
var javascriptFolds string

//go:embed nvim-treesitter/queries/javascript/highlights.scm
var javascriptHighlights string

//go:embed nvim-treesitter/queries/javascript/indents.scm
var javascriptIdents string

//go:embed nvim-treesitter/queries/javascript/injections.scm
var javascriptInjections string

//go:embed nvim-treesitter/queries/javascript/locals.scm
var javascriptLocals string

// Language jq scm embeds
//
//go:embed nvim-treesitter/queries/jq/highlights.scm
var jqHighlights string

//go:embed nvim-treesitter/queries/jq/injections.scm
var jqInjections string

// Language jsdoc scm embeds
//
//go:embed nvim-treesitter/queries/jsdoc/highlights.scm
var jsdocHighlights string

// Language json scm embeds
//
//go:embed nvim-treesitter/queries/json/folds.scm
var jsonFolds string

//go:embed nvim-treesitter/queries/json/highlights.scm
var jsonHighlights string

//go:embed nvim-treesitter/queries/json/indents.scm
var jsonIdents string

//go:embed nvim-treesitter/queries/json/locals.scm
var jsonLocals string

// Language json5 scm embeds
//
//go:embed nvim-treesitter/queries/json5/highlights.scm
var json5Highlights string

//go:embed nvim-treesitter/queries/json5/injections.scm
var json5Injections string

// Language jsonc scm embeds
//
//go:embed nvim-treesitter/queries/jsonc/folds.scm
var jsoncFolds string

//go:embed nvim-treesitter/queries/jsonc/highlights.scm
var jsoncHighlights string

//go:embed nvim-treesitter/queries/jsonc/indents.scm
var jsoncIdents string

//go:embed nvim-treesitter/queries/jsonc/injections.scm
var jsoncInjections string

//go:embed nvim-treesitter/queries/jsonc/locals.scm
var jsoncLocals string

// Language jsonnet scm embeds
//
//go:embed nvim-treesitter/queries/jsonnet/highlights.scm
var jsonnetHighlights string

// Language jsx scm embeds
//
//go:embed nvim-treesitter/queries/jsx/folds.scm
var jsxFolds string

//go:embed nvim-treesitter/queries/jsx/highlights.scm
var jsxHighlights string

//go:embed nvim-treesitter/queries/jsx/indents.scm
var jsxIdents string

//go:embed nvim-treesitter/queries/jsx/injections.scm
var jsxInjections string

// Language julia scm embeds
//
//go:embed nvim-treesitter/queries/julia/folds.scm
var juliaFolds string

//go:embed nvim-treesitter/queries/julia/highlights.scm
var juliaHighlights string

//go:embed nvim-treesitter/queries/julia/indents.scm
var juliaIdents string

//go:embed nvim-treesitter/queries/julia/injections.scm
var juliaInjections string

//go:embed nvim-treesitter/queries/julia/locals.scm
var juliaLocals string

// Language kdl scm embeds
//
//go:embed nvim-treesitter/queries/kdl/folds.scm
var kdlFolds string

//go:embed nvim-treesitter/queries/kdl/highlights.scm
var kdlHighlights string

//go:embed nvim-treesitter/queries/kdl/indents.scm
var kdlIdents string

//go:embed nvim-treesitter/queries/kdl/injections.scm
var kdlInjections string

//go:embed nvim-treesitter/queries/kdl/locals.scm
var kdlLocals string

// Language kotlin scm embeds
//
//go:embed nvim-treesitter/queries/kotlin/folds.scm
var kotlinFolds string

//go:embed nvim-treesitter/queries/kotlin/highlights.scm
var kotlinHighlights string

//go:embed nvim-treesitter/queries/kotlin/injections.scm
var kotlinInjections string

//go:embed nvim-treesitter/queries/kotlin/locals.scm
var kotlinLocals string

// Language lalrpop scm embeds
//
//go:embed nvim-treesitter/queries/lalrpop/highlights.scm
var lalrpopHighlights string

//go:embed nvim-treesitter/queries/lalrpop/injections.scm
var lalrpopInjections string

//go:embed nvim-treesitter/queries/lalrpop/locals.scm
var lalrpopLocals string

// Language latex scm embeds
//
//go:embed nvim-treesitter/queries/latex/folds.scm
var latexFolds string

//go:embed nvim-treesitter/queries/latex/highlights.scm
var latexHighlights string

//go:embed nvim-treesitter/queries/latex/injections.scm
var latexInjections string

// Language ledger scm embeds
//
//go:embed nvim-treesitter/queries/ledger/folds.scm
var ledgerFolds string

//go:embed nvim-treesitter/queries/ledger/highlights.scm
var ledgerHighlights string

//go:embed nvim-treesitter/queries/ledger/indents.scm
var ledgerIdents string

//go:embed nvim-treesitter/queries/ledger/injections.scm
var ledgerInjections string

// Language llvm scm embeds
//
//go:embed nvim-treesitter/queries/llvm/highlights.scm
var llvmHighlights string

// Language lua scm embeds
//
//go:embed nvim-treesitter/queries/lua/folds.scm
var luaFolds string

//go:embed nvim-treesitter/queries/lua/highlights.scm
var luaHighlights string

//go:embed nvim-treesitter/queries/lua/indents.scm
var luaIdents string

//go:embed nvim-treesitter/queries/lua/injections.scm
var luaInjections string

//go:embed nvim-treesitter/queries/lua/locals.scm
var luaLocals string

// Language luadoc scm embeds
//
//go:embed nvim-treesitter/queries/luadoc/highlights.scm
var luadocHighlights string

// Language luap scm embeds
//
//go:embed nvim-treesitter/queries/luap/highlights.scm
var luapHighlights string

// Language luau scm embeds
//
//go:embed nvim-treesitter/queries/luau/folds.scm
var luauFolds string

//go:embed nvim-treesitter/queries/luau/highlights.scm
var luauHighlights string

//go:embed nvim-treesitter/queries/luau/indents.scm
var luauIdents string

//go:embed nvim-treesitter/queries/luau/injections.scm
var luauInjections string

//go:embed nvim-treesitter/queries/luau/locals.scm
var luauLocals string

// Language m68k scm embeds
//
//go:embed nvim-treesitter/queries/m68k/folds.scm
var m68kFolds string

//go:embed nvim-treesitter/queries/m68k/highlights.scm
var m68kHighlights string

//go:embed nvim-treesitter/queries/m68k/injections.scm
var m68kInjections string

//go:embed nvim-treesitter/queries/m68k/locals.scm
var m68kLocals string

// Language make scm embeds
//
//go:embed nvim-treesitter/queries/make/folds.scm
var makeFolds string

//go:embed nvim-treesitter/queries/make/highlights.scm
var makeHighlights string

//go:embed nvim-treesitter/queries/make/injections.scm
var makeInjections string

// Language markdown scm embeds
//
//go:embed nvim-treesitter/queries/markdown/folds.scm
var markdownFolds string

//go:embed nvim-treesitter/queries/markdown/highlights.scm
var markdownHighlights string

//go:embed nvim-treesitter/queries/markdown/indents.scm
var markdownIdents string

//go:embed nvim-treesitter/queries/markdown/injections.scm
var markdownInjections string

// Language markdown_inline scm embeds
//
//go:embed nvim-treesitter/queries/markdown_inline/highlights.scm
var markdown_inlineHighlights string

//go:embed nvim-treesitter/queries/markdown_inline/injections.scm
var markdown_inlineInjections string

// Language matlab scm embeds
//
//go:embed nvim-treesitter/queries/matlab/folds.scm
var matlabFolds string

//go:embed nvim-treesitter/queries/matlab/highlights.scm
var matlabHighlights string

//go:embed nvim-treesitter/queries/matlab/indents.scm
var matlabIdents string

//go:embed nvim-treesitter/queries/matlab/injections.scm
var matlabInjections string

//go:embed nvim-treesitter/queries/matlab/locals.scm
var matlabLocals string

// Language menhir scm embeds
//
//go:embed nvim-treesitter/queries/menhir/highlights.scm
var menhirHighlights string

//go:embed nvim-treesitter/queries/menhir/injections.scm
var menhirInjections string

// Language mermaid scm embeds
//
//go:embed nvim-treesitter/queries/mermaid/highlights.scm
var mermaidHighlights string

// Language meson scm embeds
//
//go:embed nvim-treesitter/queries/meson/folds.scm
var mesonFolds string

//go:embed nvim-treesitter/queries/meson/highlights.scm
var mesonHighlights string

//go:embed nvim-treesitter/queries/meson/injections.scm
var mesonInjections string

// Language mlir scm embeds
//
//go:embed nvim-treesitter/queries/mlir/highlights.scm
var mlirHighlights string

//go:embed nvim-treesitter/queries/mlir/locals.scm
var mlirLocals string

// Language nickel scm embeds
//
//go:embed nvim-treesitter/queries/nickel/highlights.scm
var nickelHighlights string

//go:embed nvim-treesitter/queries/nickel/indents.scm
var nickelIdents string

// Language ninja scm embeds
//
//go:embed nvim-treesitter/queries/ninja/folds.scm
var ninjaFolds string

//go:embed nvim-treesitter/queries/ninja/highlights.scm
var ninjaHighlights string

//go:embed nvim-treesitter/queries/ninja/indents.scm
var ninjaIdents string

// Language nix scm embeds
//
//go:embed nvim-treesitter/queries/nix/folds.scm
var nixFolds string

//go:embed nvim-treesitter/queries/nix/highlights.scm
var nixHighlights string

//go:embed nvim-treesitter/queries/nix/injections.scm
var nixInjections string

//go:embed nvim-treesitter/queries/nix/locals.scm
var nixLocals string

// Language objc scm embeds
//
//go:embed nvim-treesitter/queries/objc/folds.scm
var objcFolds string

//go:embed nvim-treesitter/queries/objc/highlights.scm
var objcHighlights string

//go:embed nvim-treesitter/queries/objc/indents.scm
var objcIdents string

//go:embed nvim-treesitter/queries/objc/injections.scm
var objcInjections string

//go:embed nvim-treesitter/queries/objc/locals.scm
var objcLocals string

// Language ocaml scm embeds
//
//go:embed nvim-treesitter/queries/ocaml/folds.scm
var ocamlFolds string

//go:embed nvim-treesitter/queries/ocaml/highlights.scm
var ocamlHighlights string

//go:embed nvim-treesitter/queries/ocaml/indents.scm
var ocamlIdents string

//go:embed nvim-treesitter/queries/ocaml/injections.scm
var ocamlInjections string

//go:embed nvim-treesitter/queries/ocaml/locals.scm
var ocamlLocals string

// Language ocaml_interface scm embeds
//
//go:embed nvim-treesitter/queries/ocaml_interface/folds.scm
var ocaml_interfaceFolds string

//go:embed nvim-treesitter/queries/ocaml_interface/highlights.scm
var ocaml_interfaceHighlights string

//go:embed nvim-treesitter/queries/ocaml_interface/indents.scm
var ocaml_interfaceIdents string

//go:embed nvim-treesitter/queries/ocaml_interface/injections.scm
var ocaml_interfaceInjections string

//go:embed nvim-treesitter/queries/ocaml_interface/locals.scm
var ocaml_interfaceLocals string

// Language ocamllex scm embeds
//
//go:embed nvim-treesitter/queries/ocamllex/highlights.scm
var ocamllexHighlights string

//go:embed nvim-treesitter/queries/ocamllex/injections.scm
var ocamllexInjections string

// Language odin scm embeds
//
//go:embed nvim-treesitter/queries/odin/folds.scm
var odinFolds string

//go:embed nvim-treesitter/queries/odin/highlights.scm
var odinHighlights string

//go:embed nvim-treesitter/queries/odin/indents.scm
var odinIdents string

//go:embed nvim-treesitter/queries/odin/injections.scm
var odinInjections string

//go:embed nvim-treesitter/queries/odin/locals.scm
var odinLocals string

// Language pascal scm embeds
//
//go:embed nvim-treesitter/queries/pascal/folds.scm
var pascalFolds string

//go:embed nvim-treesitter/queries/pascal/highlights.scm
var pascalHighlights string

//go:embed nvim-treesitter/queries/pascal/indents.scm
var pascalIdents string

//go:embed nvim-treesitter/queries/pascal/injections.scm
var pascalInjections string

//go:embed nvim-treesitter/queries/pascal/locals.scm
var pascalLocals string

// Language passwd scm embeds
//
//go:embed nvim-treesitter/queries/passwd/highlights.scm
var passwdHighlights string

// Language perl scm embeds
//
//go:embed nvim-treesitter/queries/perl/folds.scm
var perlFolds string

//go:embed nvim-treesitter/queries/perl/highlights.scm
var perlHighlights string

//go:embed nvim-treesitter/queries/perl/injections.scm
var perlInjections string

// Language php scm embeds
//
//go:embed nvim-treesitter/queries/php/folds.scm
var phpFolds string

//go:embed nvim-treesitter/queries/php/highlights.scm
var phpHighlights string

//go:embed nvim-treesitter/queries/php/indents.scm
var phpIdents string

//go:embed nvim-treesitter/queries/php/injections.scm
var phpInjections string

//go:embed nvim-treesitter/queries/php/locals.scm
var phpLocals string

// Language phpdoc scm embeds
//
//go:embed nvim-treesitter/queries/phpdoc/highlights.scm
var phpdocHighlights string

// Language pioasm scm embeds
//
//go:embed nvim-treesitter/queries/pioasm/highlights.scm
var pioasmHighlights string

//go:embed nvim-treesitter/queries/pioasm/injections.scm
var pioasmInjections string

// Language po scm embeds
//
//go:embed nvim-treesitter/queries/po/folds.scm
var poFolds string

//go:embed nvim-treesitter/queries/po/highlights.scm
var poHighlights string

//go:embed nvim-treesitter/queries/po/injections.scm
var poInjections string

// Language poe_filter scm embeds
//
//go:embed nvim-treesitter/queries/poe_filter/folds.scm
var poe_filterFolds string

//go:embed nvim-treesitter/queries/poe_filter/highlights.scm
var poe_filterHighlights string

//go:embed nvim-treesitter/queries/poe_filter/indents.scm
var poe_filterIdents string

//go:embed nvim-treesitter/queries/poe_filter/injections.scm
var poe_filterInjections string

// Language pony scm embeds
//
//go:embed nvim-treesitter/queries/pony/folds.scm
var ponyFolds string

//go:embed nvim-treesitter/queries/pony/highlights.scm
var ponyHighlights string

//go:embed nvim-treesitter/queries/pony/indents.scm
var ponyIdents string

//go:embed nvim-treesitter/queries/pony/injections.scm
var ponyInjections string

//go:embed nvim-treesitter/queries/pony/locals.scm
var ponyLocals string

// Language prisma scm embeds
//
//go:embed nvim-treesitter/queries/prisma/highlights.scm
var prismaHighlights string

// Language proto scm embeds
//
//go:embed nvim-treesitter/queries/proto/folds.scm
var protoFolds string

//go:embed nvim-treesitter/queries/proto/highlights.scm
var protoHighlights string

// Language prql scm embeds
//
//go:embed nvim-treesitter/queries/prql/highlights.scm
var prqlHighlights string

//go:embed nvim-treesitter/queries/prql/injections.scm
var prqlInjections string

// Language pug scm embeds
//
//go:embed nvim-treesitter/queries/pug/highlights.scm
var pugHighlights string

//go:embed nvim-treesitter/queries/pug/injections.scm
var pugInjections string

// Language puppet scm embeds
//
//go:embed nvim-treesitter/queries/puppet/folds.scm
var puppetFolds string

//go:embed nvim-treesitter/queries/puppet/highlights.scm
var puppetHighlights string

//go:embed nvim-treesitter/queries/puppet/indents.scm
var puppetIdents string

//go:embed nvim-treesitter/queries/puppet/injections.scm
var puppetInjections string

//go:embed nvim-treesitter/queries/puppet/locals.scm
var puppetLocals string

// Language python scm embeds
//
//go:embed nvim-treesitter/queries/python/folds.scm
var pythonFolds string

//go:embed nvim-treesitter/queries/python/highlights.scm
var pythonHighlights string

//go:embed nvim-treesitter/queries/python/indents.scm
var pythonIdents string

//go:embed nvim-treesitter/queries/python/injections.scm
var pythonInjections string

//go:embed nvim-treesitter/queries/python/locals.scm
var pythonLocals string

// Language ql scm embeds
//
//go:embed nvim-treesitter/queries/ql/folds.scm
var qlFolds string

//go:embed nvim-treesitter/queries/ql/highlights.scm
var qlHighlights string

//go:embed nvim-treesitter/queries/ql/indents.scm
var qlIdents string

//go:embed nvim-treesitter/queries/ql/injections.scm
var qlInjections string

//go:embed nvim-treesitter/queries/ql/locals.scm
var qlLocals string

// Language qmldir scm embeds
//
//go:embed nvim-treesitter/queries/qmldir/highlights.scm
var qmldirHighlights string

//go:embed nvim-treesitter/queries/qmldir/injections.scm
var qmldirInjections string

// Language qmljs scm embeds
//
//go:embed nvim-treesitter/queries/qmljs/folds.scm
var qmljsFolds string

//go:embed nvim-treesitter/queries/qmljs/highlights.scm
var qmljsHighlights string

// Language query scm embeds
//
//go:embed nvim-treesitter/queries/query/folds.scm
var queryFolds string

//go:embed nvim-treesitter/queries/query/highlights.scm
var queryHighlights string

//go:embed nvim-treesitter/queries/query/indents.scm
var queryIdents string

//go:embed nvim-treesitter/queries/query/injections.scm
var queryInjections string

//go:embed nvim-treesitter/queries/query/locals.scm
var queryLocals string

// Language r scm embeds
//
//go:embed nvim-treesitter/queries/r/highlights.scm
var rHighlights string

//go:embed nvim-treesitter/queries/r/indents.scm
var rIdents string

//go:embed nvim-treesitter/queries/r/injections.scm
var rInjections string

//go:embed nvim-treesitter/queries/r/locals.scm
var rLocals string

// Language racket scm embeds
//
//go:embed nvim-treesitter/queries/racket/folds.scm
var racketFolds string

//go:embed nvim-treesitter/queries/racket/highlights.scm
var racketHighlights string

//go:embed nvim-treesitter/queries/racket/injections.scm
var racketInjections string

// Language rasi scm embeds
//
//go:embed nvim-treesitter/queries/rasi/folds.scm
var rasiFolds string

//go:embed nvim-treesitter/queries/rasi/highlights.scm
var rasiHighlights string

//go:embed nvim-treesitter/queries/rasi/indents.scm
var rasiIdents string

//go:embed nvim-treesitter/queries/rasi/locals.scm
var rasiLocals string

// Language regex scm embeds
//
//go:embed nvim-treesitter/queries/regex/highlights.scm
var regexHighlights string

// Language rego scm embeds
//
//go:embed nvim-treesitter/queries/rego/highlights.scm
var regoHighlights string

//go:embed nvim-treesitter/queries/rego/injections.scm
var regoInjections string

// Language rnoweb scm embeds
//
//go:embed nvim-treesitter/queries/rnoweb/folds.scm
var rnowebFolds string

//go:embed nvim-treesitter/queries/rnoweb/highlights.scm
var rnowebHighlights string

//go:embed nvim-treesitter/queries/rnoweb/injections.scm
var rnowebInjections string

// Language ron scm embeds
//
//go:embed nvim-treesitter/queries/ron/folds.scm
var ronFolds string

//go:embed nvim-treesitter/queries/ron/highlights.scm
var ronHighlights string

//go:embed nvim-treesitter/queries/ron/indents.scm
var ronIdents string

//go:embed nvim-treesitter/queries/ron/injections.scm
var ronInjections string

//go:embed nvim-treesitter/queries/ron/locals.scm
var ronLocals string

// Language rst scm embeds
//
//go:embed nvim-treesitter/queries/rst/highlights.scm
var rstHighlights string

//go:embed nvim-treesitter/queries/rst/injections.scm
var rstInjections string

//go:embed nvim-treesitter/queries/rst/locals.scm
var rstLocals string

// Language ruby scm embeds
//
//go:embed nvim-treesitter/queries/ruby/folds.scm
var rubyFolds string

//go:embed nvim-treesitter/queries/ruby/highlights.scm
var rubyHighlights string

//go:embed nvim-treesitter/queries/ruby/indents.scm
var rubyIdents string

//go:embed nvim-treesitter/queries/ruby/injections.scm
var rubyInjections string

//go:embed nvim-treesitter/queries/ruby/locals.scm
var rubyLocals string

// Language rust scm embeds
//
//go:embed nvim-treesitter/queries/rust/folds.scm
var rustFolds string

//go:embed nvim-treesitter/queries/rust/highlights.scm
var rustHighlights string

//go:embed nvim-treesitter/queries/rust/indents.scm
var rustIdents string

//go:embed nvim-treesitter/queries/rust/injections.scm
var rustInjections string

//go:embed nvim-treesitter/queries/rust/locals.scm
var rustLocals string

// Language scala scm embeds
//
//go:embed nvim-treesitter/queries/scala/folds.scm
var scalaFolds string

//go:embed nvim-treesitter/queries/scala/highlights.scm
var scalaHighlights string

//go:embed nvim-treesitter/queries/scala/injections.scm
var scalaInjections string

//go:embed nvim-treesitter/queries/scala/locals.scm
var scalaLocals string

// Language scheme scm embeds
//
//go:embed nvim-treesitter/queries/scheme/folds.scm
var schemeFolds string

//go:embed nvim-treesitter/queries/scheme/highlights.scm
var schemeHighlights string

//go:embed nvim-treesitter/queries/scheme/injections.scm
var schemeInjections string

// Language scss scm embeds
//
//go:embed nvim-treesitter/queries/scss/folds.scm
var scssFolds string

//go:embed nvim-treesitter/queries/scss/highlights.scm
var scssHighlights string

//go:embed nvim-treesitter/queries/scss/indents.scm
var scssIdents string

// Language slint scm embeds
//
//go:embed nvim-treesitter/queries/slint/highlights.scm
var slintHighlights string

//go:embed nvim-treesitter/queries/slint/indents.scm
var slintIdents string

// Language smali scm embeds
//
//go:embed nvim-treesitter/queries/smali/folds.scm
var smaliFolds string

//go:embed nvim-treesitter/queries/smali/highlights.scm
var smaliHighlights string

//go:embed nvim-treesitter/queries/smali/indents.scm
var smaliIdents string

//go:embed nvim-treesitter/queries/smali/injections.scm
var smaliInjections string

//go:embed nvim-treesitter/queries/smali/locals.scm
var smaliLocals string

// Language smithy scm embeds
//
//go:embed nvim-treesitter/queries/smithy/highlights.scm
var smithyHighlights string

// Language solidity scm embeds
//
//go:embed nvim-treesitter/queries/solidity/highlights.scm
var solidityHighlights string

// Language sparql scm embeds
//
//go:embed nvim-treesitter/queries/sparql/folds.scm
var sparqlFolds string

//go:embed nvim-treesitter/queries/sparql/highlights.scm
var sparqlHighlights string

//go:embed nvim-treesitter/queries/sparql/indents.scm
var sparqlIdents string

//go:embed nvim-treesitter/queries/sparql/injections.scm
var sparqlInjections string

//go:embed nvim-treesitter/queries/sparql/locals.scm
var sparqlLocals string

// Language sql scm embeds
//
//go:embed nvim-treesitter/queries/sql/highlights.scm
var sqlHighlights string

//go:embed nvim-treesitter/queries/sql/indents.scm
var sqlIdents string

//go:embed nvim-treesitter/queries/sql/injections.scm
var sqlInjections string

// Language squirrel scm embeds
//
//go:embed nvim-treesitter/queries/squirrel/folds.scm
var squirrelFolds string

//go:embed nvim-treesitter/queries/squirrel/highlights.scm
var squirrelHighlights string

//go:embed nvim-treesitter/queries/squirrel/indents.scm
var squirrelIdents string

//go:embed nvim-treesitter/queries/squirrel/injections.scm
var squirrelInjections string

//go:embed nvim-treesitter/queries/squirrel/locals.scm
var squirrelLocals string

// Language starlark scm embeds
//
//go:embed nvim-treesitter/queries/starlark/folds.scm
var starlarkFolds string

//go:embed nvim-treesitter/queries/starlark/highlights.scm
var starlarkHighlights string

//go:embed nvim-treesitter/queries/starlark/indents.scm
var starlarkIdents string

//go:embed nvim-treesitter/queries/starlark/injections.scm
var starlarkInjections string

//go:embed nvim-treesitter/queries/starlark/locals.scm
var starlarkLocals string

// Language supercollider scm embeds
//
//go:embed nvim-treesitter/queries/supercollider/folds.scm
var supercolliderFolds string

//go:embed nvim-treesitter/queries/supercollider/highlights.scm
var supercolliderHighlights string

//go:embed nvim-treesitter/queries/supercollider/indents.scm
var supercolliderIdents string

//go:embed nvim-treesitter/queries/supercollider/injections.scm
var supercolliderInjections string

//go:embed nvim-treesitter/queries/supercollider/locals.scm
var supercolliderLocals string

// Language surface scm embeds
//
//go:embed nvim-treesitter/queries/surface/folds.scm
var surfaceFolds string

//go:embed nvim-treesitter/queries/surface/highlights.scm
var surfaceHighlights string

//go:embed nvim-treesitter/queries/surface/indents.scm
var surfaceIdents string

//go:embed nvim-treesitter/queries/surface/injections.scm
var surfaceInjections string

// Language svelte scm embeds
//
//go:embed nvim-treesitter/queries/svelte/folds.scm
var svelteFolds string

//go:embed nvim-treesitter/queries/svelte/highlights.scm
var svelteHighlights string

//go:embed nvim-treesitter/queries/svelte/indents.scm
var svelteIdents string

//go:embed nvim-treesitter/queries/svelte/injections.scm
var svelteInjections string

// Language swift scm embeds
//
//go:embed nvim-treesitter/queries/swift/highlights.scm
var swiftHighlights string

//go:embed nvim-treesitter/queries/swift/indents.scm
var swiftIdents string

//go:embed nvim-treesitter/queries/swift/locals.scm
var swiftLocals string

// Language sxhkdrc scm embeds
//
//go:embed nvim-treesitter/queries/sxhkdrc/folds.scm
var sxhkdrcFolds string

//go:embed nvim-treesitter/queries/sxhkdrc/highlights.scm
var sxhkdrcHighlights string

//go:embed nvim-treesitter/queries/sxhkdrc/injections.scm
var sxhkdrcInjections string

// Language t32 scm embeds
//
//go:embed nvim-treesitter/queries/t32/folds.scm
var t32Folds string

//go:embed nvim-treesitter/queries/t32/highlights.scm
var t32Highlights string

//go:embed nvim-treesitter/queries/t32/indents.scm
var t32Idents string

//go:embed nvim-treesitter/queries/t32/injections.scm
var t32Injections string

//go:embed nvim-treesitter/queries/t32/locals.scm
var t32Locals string

// Language tablegen scm embeds
//
//go:embed nvim-treesitter/queries/tablegen/folds.scm
var tablegenFolds string

//go:embed nvim-treesitter/queries/tablegen/highlights.scm
var tablegenHighlights string

//go:embed nvim-treesitter/queries/tablegen/indents.scm
var tablegenIdents string

//go:embed nvim-treesitter/queries/tablegen/injections.scm
var tablegenInjections string

//go:embed nvim-treesitter/queries/tablegen/locals.scm
var tablegenLocals string

// Language teal scm embeds
//
//go:embed nvim-treesitter/queries/teal/folds.scm
var tealFolds string

//go:embed nvim-treesitter/queries/teal/highlights.scm
var tealHighlights string

//go:embed nvim-treesitter/queries/teal/indents.scm
var tealIdents string

//go:embed nvim-treesitter/queries/teal/injections.scm
var tealInjections string

//go:embed nvim-treesitter/queries/teal/locals.scm
var tealLocals string

// Language terraform scm embeds
//
//go:embed nvim-treesitter/queries/terraform/folds.scm
var terraformFolds string

//go:embed nvim-treesitter/queries/terraform/highlights.scm
var terraformHighlights string

//go:embed nvim-treesitter/queries/terraform/indents.scm
var terraformIdents string

//go:embed nvim-treesitter/queries/terraform/injections.scm
var terraformInjections string

// Language thrift scm embeds
//
//go:embed nvim-treesitter/queries/thrift/folds.scm
var thriftFolds string

//go:embed nvim-treesitter/queries/thrift/highlights.scm
var thriftHighlights string

//go:embed nvim-treesitter/queries/thrift/indents.scm
var thriftIdents string

//go:embed nvim-treesitter/queries/thrift/injections.scm
var thriftInjections string

//go:embed nvim-treesitter/queries/thrift/locals.scm
var thriftLocals string

// Language tiger scm embeds
//
//go:embed nvim-treesitter/queries/tiger/folds.scm
var tigerFolds string

//go:embed nvim-treesitter/queries/tiger/highlights.scm
var tigerHighlights string

//go:embed nvim-treesitter/queries/tiger/indents.scm
var tigerIdents string

//go:embed nvim-treesitter/queries/tiger/injections.scm
var tigerInjections string

//go:embed nvim-treesitter/queries/tiger/locals.scm
var tigerLocals string

// Language tlaplus scm embeds
//
//go:embed nvim-treesitter/queries/tlaplus/folds.scm
var tlaplusFolds string

//go:embed nvim-treesitter/queries/tlaplus/highlights.scm
var tlaplusHighlights string

//go:embed nvim-treesitter/queries/tlaplus/injections.scm
var tlaplusInjections string

//go:embed nvim-treesitter/queries/tlaplus/locals.scm
var tlaplusLocals string

// Language todotxt scm embeds
//
//go:embed nvim-treesitter/queries/todotxt/highlights.scm
var todotxtHighlights string

// Language toml scm embeds
//
//go:embed nvim-treesitter/queries/toml/folds.scm
var tomlFolds string

//go:embed nvim-treesitter/queries/toml/highlights.scm
var tomlHighlights string

//go:embed nvim-treesitter/queries/toml/indents.scm
var tomlIdents string

//go:embed nvim-treesitter/queries/toml/injections.scm
var tomlInjections string

//go:embed nvim-treesitter/queries/toml/locals.scm
var tomlLocals string

// Language tsx scm embeds
//
//go:embed nvim-treesitter/queries/tsx/folds.scm
var tsxFolds string

//go:embed nvim-treesitter/queries/tsx/highlights.scm
var tsxHighlights string

//go:embed nvim-treesitter/queries/tsx/indents.scm
var tsxIdents string

//go:embed nvim-treesitter/queries/tsx/injections.scm
var tsxInjections string

//go:embed nvim-treesitter/queries/tsx/locals.scm
var tsxLocals string

// Language turtle scm embeds
//
//go:embed nvim-treesitter/queries/turtle/folds.scm
var turtleFolds string

//go:embed nvim-treesitter/queries/turtle/highlights.scm
var turtleHighlights string

//go:embed nvim-treesitter/queries/turtle/indents.scm
var turtleIdents string

//go:embed nvim-treesitter/queries/turtle/injections.scm
var turtleInjections string

//go:embed nvim-treesitter/queries/turtle/locals.scm
var turtleLocals string

// Language twig scm embeds
//
//go:embed nvim-treesitter/queries/twig/highlights.scm
var twigHighlights string

//go:embed nvim-treesitter/queries/twig/injections.scm
var twigInjections string

// Language typescript scm embeds
//
//go:embed nvim-treesitter/queries/typescript/folds.scm
var typescriptFolds string

//go:embed nvim-treesitter/queries/typescript/highlights.scm
var typescriptHighlights string

//go:embed nvim-treesitter/queries/typescript/indents.scm
var typescriptIdents string

//go:embed nvim-treesitter/queries/typescript/injections.scm
var typescriptInjections string

//go:embed nvim-treesitter/queries/typescript/locals.scm
var typescriptLocals string

// Language ungrammar scm embeds
//
//go:embed nvim-treesitter/queries/ungrammar/folds.scm
var ungrammarFolds string

//go:embed nvim-treesitter/queries/ungrammar/highlights.scm
var ungrammarHighlights string

//go:embed nvim-treesitter/queries/ungrammar/indents.scm
var ungrammarIdents string

//go:embed nvim-treesitter/queries/ungrammar/injections.scm
var ungrammarInjections string

//go:embed nvim-treesitter/queries/ungrammar/locals.scm
var ungrammarLocals string

// Language usd scm embeds
//
//go:embed nvim-treesitter/queries/usd/folds.scm
var usdFolds string

//go:embed nvim-treesitter/queries/usd/highlights.scm
var usdHighlights string

//go:embed nvim-treesitter/queries/usd/indents.scm
var usdIdents string

//go:embed nvim-treesitter/queries/usd/locals.scm
var usdLocals string

// Language uxntal scm embeds
//
//go:embed nvim-treesitter/queries/uxntal/folds.scm
var uxntalFolds string

//go:embed nvim-treesitter/queries/uxntal/highlights.scm
var uxntalHighlights string

//go:embed nvim-treesitter/queries/uxntal/indents.scm
var uxntalIdents string

//go:embed nvim-treesitter/queries/uxntal/injections.scm
var uxntalInjections string

//go:embed nvim-treesitter/queries/uxntal/locals.scm
var uxntalLocals string

// Language v scm embeds
//
//go:embed nvim-treesitter/queries/v/folds.scm
var vFolds string

//go:embed nvim-treesitter/queries/v/highlights.scm
var vHighlights string

//go:embed nvim-treesitter/queries/v/indents.scm
var vIdents string

//go:embed nvim-treesitter/queries/v/injections.scm
var vInjections string

//go:embed nvim-treesitter/queries/v/locals.scm
var vLocals string

// Language vala scm embeds
//
//go:embed nvim-treesitter/queries/vala/folds.scm
var valaFolds string

//go:embed nvim-treesitter/queries/vala/highlights.scm
var valaHighlights string

// Language verilog scm embeds
//
//go:embed nvim-treesitter/queries/verilog/folds.scm
var verilogFolds string

//go:embed nvim-treesitter/queries/verilog/highlights.scm
var verilogHighlights string

//go:embed nvim-treesitter/queries/verilog/injections.scm
var verilogInjections string

//go:embed nvim-treesitter/queries/verilog/locals.scm
var verilogLocals string

// Language vhs scm embeds
//
//go:embed nvim-treesitter/queries/vhs/highlights.scm
var vhsHighlights string

// Language vim scm embeds
//
//go:embed nvim-treesitter/queries/vim/folds.scm
var vimFolds string

//go:embed nvim-treesitter/queries/vim/highlights.scm
var vimHighlights string

//go:embed nvim-treesitter/queries/vim/injections.scm
var vimInjections string

//go:embed nvim-treesitter/queries/vim/locals.scm
var vimLocals string

// Language vimdoc scm embeds
//
//go:embed nvim-treesitter/queries/vimdoc/highlights.scm
var vimdocHighlights string

//go:embed nvim-treesitter/queries/vimdoc/injections.scm
var vimdocInjections string

// Language vue scm embeds
//
//go:embed nvim-treesitter/queries/vue/folds.scm
var vueFolds string

//go:embed nvim-treesitter/queries/vue/highlights.scm
var vueHighlights string

//go:embed nvim-treesitter/queries/vue/indents.scm
var vueIdents string

//go:embed nvim-treesitter/queries/vue/injections.scm
var vueInjections string

// Language wgsl scm embeds
//
//go:embed nvim-treesitter/queries/wgsl/folds.scm
var wgslFolds string

//go:embed nvim-treesitter/queries/wgsl/highlights.scm
var wgslHighlights string

//go:embed nvim-treesitter/queries/wgsl/indents.scm
var wgslIdents string

// Language wgsl_bevy scm embeds
//
//go:embed nvim-treesitter/queries/wgsl_bevy/folds.scm
var wgsl_bevyFolds string

//go:embed nvim-treesitter/queries/wgsl_bevy/highlights.scm
var wgsl_bevyHighlights string

//go:embed nvim-treesitter/queries/wgsl_bevy/indents.scm
var wgsl_bevyIdents string

// Language yaml scm embeds
//
//go:embed nvim-treesitter/queries/yaml/folds.scm
var yamlFolds string

//go:embed nvim-treesitter/queries/yaml/highlights.scm
var yamlHighlights string

//go:embed nvim-treesitter/queries/yaml/indents.scm
var yamlIdents string

//go:embed nvim-treesitter/queries/yaml/injections.scm
var yamlInjections string

//go:embed nvim-treesitter/queries/yaml/locals.scm
var yamlLocals string

// Language yang scm embeds
//
//go:embed nvim-treesitter/queries/yang/folds.scm
var yangFolds string

//go:embed nvim-treesitter/queries/yang/highlights.scm
var yangHighlights string

//go:embed nvim-treesitter/queries/yang/indents.scm
var yangIdents string

//go:embed nvim-treesitter/queries/yang/injections.scm
var yangInjections string

// Language yuck scm embeds
//
//go:embed nvim-treesitter/queries/yuck/folds.scm
var yuckFolds string

//go:embed nvim-treesitter/queries/yuck/highlights.scm
var yuckHighlights string

//go:embed nvim-treesitter/queries/yuck/indents.scm
var yuckIdents string

//go:embed nvim-treesitter/queries/yuck/injections.scm
var yuckInjections string

//go:embed nvim-treesitter/queries/yuck/locals.scm
var yuckLocals string

// Language zig scm embeds
//
//go:embed nvim-treesitter/queries/zig/folds.scm
var zigFolds string

//go:embed nvim-treesitter/queries/zig/highlights.scm
var zigHighlights string

//go:embed nvim-treesitter/queries/zig/indents.scm
var zigIdents string

//go:embed nvim-treesitter/queries/zig/injections.scm
var zigInjections string

//go:embed nvim-treesitter/queries/zig/locals.scm
var zigLocals string
