module unstable.build/go-tui

go 1.25.6

require (
	github.com/atotto/clipboard v0.1.2
	github.com/disintegration/imaging v1.6.2
	github.com/ernestrc/dom v0.2.5-0.20201125033726-789c946aa938
	github.com/ernestrc/go-multierror v1.1.2
	github.com/ernestrc/sensible v0.3.1
	github.com/junegunn/fzf v0.0.0-20201216124428-ab3937ee5a62
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/pion/mediadevices v0.6.2
	github.com/pion/webrtc/v3 v3.2.37
	github.com/sirupsen/logrus v1.9.3
	github.com/sourcegraph/go-diff v0.6.1
	github.com/stretchr/testify v1.11.1
	github.com/unstablebuild/blue v1.62.0
	github.com/unstablebuild/golang-internal-tools v0.0.2
	github.com/unstablebuild/rune-go-sdk v0.0.2
	github.com/unstablebuild/tcell/v3 v3.6.1
	go.uber.org/goleak v1.3.0
	golang.org/x/crypto v0.46.0
	golang.org/x/image v0.16.0
	golang.org/x/term v0.39.0
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/ernestrc/logd-go v0.0.0-20180509171507-65871c1d5504
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)

require (
	github.com/ebitengine/purego v0.9.0
	github.com/go-git/go-billy/v6 v6.0.0-20251022185412-61e52df296a5
	github.com/go-git/go-git/v6 v6.0.0-20250819122726-39261590f7f3
	github.com/sergi/go-diff v1.4.0
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/unstablebuild/notify v0.10.2
	github.com/unstablebuild/pty v1.3.1
	go.uber.org/mock v0.4.0
	go.uber.org/multierr v1.4.0
	golang.org/x/oauth2 v0.34.0
	mvdan.cc/sh/v3 v3.12.0
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.3.0 // indirect
	github.com/blackjack/webcam v0.5.0 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/cyphar/filepath-securejoin v0.6.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/gen2brain/malgo v0.11.21 // indirect
	github.com/go-git/gcfg/v2 v2.0.2 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/kevinburke/ssh_config v1.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/pion/datachannel v1.5.6 // indirect
	github.com/pion/dtls/v2 v2.2.10 // indirect
	github.com/pion/ice/v2 v2.3.14 // indirect
	github.com/pion/interceptor v0.1.28 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.12 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.14 // indirect
	github.com/pion/rtp v1.8.5 // indirect
	github.com/pion/sctp v1.8.15 // indirect
	github.com/pion/sdp/v3 v3.0.9 // indirect
	github.com/pion/srtp/v2 v2.0.18 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.4 // indirect
	github.com/pion/turn/v2 v2.1.5 // indirect
	github.com/pjbgf/sha1cd v0.5.0 // indirect
	go.uber.org/atomic v1.5.0 // indirect
	go.uber.org/tools v0.0.0-20190618225709-2cfd321de3ee // indirect
	golang.org/x/exp/typeparams v0.0.0-20220722155223-a9213eeb770e // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
	golang.org/x/tools/go/packages/packagestest v0.1.1-deprecated // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	honnef.co/go/tools v0.3.2 // indirect
)

replace github.com/go-git/go-billy/v6 => github.com/unstablebuild/go-billy/v6 v6.0.0-ub.1

replace github.com/go-git/go-git/v6 => github.com/unstablebuild/go-git/v6 v6.0.1-ub.1

replace github.com/tree-sitter/go-tree-sitter => github.com/unstablebuild/go-tree-sitter v0.25.0-ub.1
