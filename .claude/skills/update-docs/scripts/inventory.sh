#!/usr/bin/env bash
# Inventory of documentable facts for the update-docs skill.
#
#   inventory.sh                  print the inventory + what changed since the last docs sync
#   inventory.sh --check          verify README.md + docs/*.md cover every env var and route (exit 1 on gaps)
#   inventory.sh --write-manifest record current source hashes as "docs are in sync" (docs/.sync-manifest)
#
# Facts come straight from the code so the docs can be checked against them.
# Defaults of secret-looking variables are never printed.
set -euo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$root"

DOCS_DIR=docs
MANIFEST="$DOCS_DIR/.sync-manifest"
SRC_EXT='\.(go|mod|json|tsv|sh|patch|xml|sql|ya?ml|toml|service|conf)$'
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

hash_file() {
  if command -v sha256sum >/dev/null; then sha256sum -- "$1"; else shasum -a 256 -- "$1"; fi
}

# Files whose changes can make the docs stale (tracked + untracked-but-not-ignored).
source_files() {
  if git rev-parse --git-dir >/dev/null 2>&1; then
    git ls-files -co --exclude-standard
  else
    find . -type f | sed 's|^\./||'
  fi | grep -E "$SRC_EXT" | grep -vE '^(docs/|\.claude/|vendor/)' \
     | while IFS= read -r f; do [ -f "$f" ] && printf '%s\n' "$f"; done | LC_ALL=C sort -u
}

go_files() { source_files | grep -E '\.go$' | grep -vE '_test\.go$' || true; }

doc_files() {
  [ -f README.md ] && echo README.md
  [ -d "$DOCS_DIR" ] && find "$DOCS_DIR" -name '*.md' -type f | LC_ALL=C sort
  true
}

# NAME <tab> default <tab> locations — one row per env var.
env_vars() {
  go_files | perl -e '
    chomp(my @files = <STDIN>);
    my (%def, %loc, @order);
    sub emit {
      my ($n, $d, $where) = @_;
      $d =~ s/\s+$//;
      $d = "<redacted>" if $n =~ /PASS|SECRET|TOKEN|KEY|PWD|CRED/i && $d ne q("") && $d !~ /^\(/;
      push @order, $n unless exists $loc{$n};
      $def{$n}{$d} = 1;
      push @{$loc{$n}}, $where;
    }
    for my $f (@files) {
      open my $fh, "<", $f or next; my @L = <$fh>; close $fh;
      for my $i (0 .. $#L) {
        my $l = $L[$i]; my %seen;
        while ($l =~ /\benvOr\(\s*"(\w+)"\s*,\s*((?:[^()]|(\((?:[^()]|(?3))*\)))*)\)/g) {
          emit($1, $2, "$f:" . ($i + 1)); $seen{$1} = 1;
        }
        while ($l =~ /\benvOr\(\s*"(\w+)"/g) {
          emit($1, "(nested default)", "$f:" . ($i + 1)) unless $seen{$1}++;
        }
        while ($l =~ /\bos\.(?:Getenv|LookupEnv)\(\s*"(\w+)"\s*\)/g) {
          my ($n, $d) = ($1, "");
          if ($l =~ /(\w+)\s*:?=\s*.*os\.(?:Getenv|LookupEnv)/) {
            my $v = $1;
            for my $j ($i + 1 .. ($i + 4 > $#L ? $#L : $i + 4)) {
              if ($L[$j] =~ /^\s*\Q$v\E\s*=\s*("(?:[^"\\]|\\.)*")/) { $d = $1; last }
            }
          }
          emit($n, $d eq "" ? "(none - see code)" : $d, "$f:" . ($i + 1));
        }
      }
    }
    for my $n (sort @order) {
      printf "%s\t%s\t%s\n", $n, join(" | ", sort keys %{$def{$n}}), join(", ", @{$loc{$n}});
    }'
}

# path <tab> access <tab> location — one row per registered HTTP route.
routes() {
  go_files | perl -e '
    chomp(my @files = <STDIN>);
    for my $f (@files) {
      open my $fh, "<", $f or next;
      while (my $l = <$fh>) {
        if (my ($path, $rest) = $l =~ /\.(?:HandleFunc|Handle)\(\s*"([^"]+)"\s*,\s*(.*)$/) {
          my $access = $rest =~ /requireAdmin/ ? "admin-login" : "open";
          print "$path\t$access\t$f:$.\n";
        }
      }
      close $fh;
    }' | LC_ALL=C sort -t$'\t' -k1,1
}

print_inventory() {
  echo "## Repo"
  echo "root: $root"
  if git rev-parse --git-dir >/dev/null 2>&1; then
    echo "branch: $(git branch --show-current 2>/dev/null || echo '?')  HEAD: $(git rev-parse --short HEAD 2>/dev/null || echo none)"
    echo "uncommitted source changes: $(git status --porcelain | awk '{print $NF}' | grep -cE "$SRC_EXT" || true)"
  fi
  [ -f go.mod ] && echo "go module: $(awk '/^module /{print $2}' go.mod)  go: $(awk '/^go /{print $2}' go.mod)"
  [ -f go.mod ] && echo "direct deps: $(awk '/^require \(/{r=1;next} r&&/^\)/{r=0} r&&!/indirect/{printf "%s %s; ", $1, $2}' go.mod)"
  echo

  echo "## Existing docs"
  local docs; docs=$(doc_files)
  if [ -z "$docs" ]; then
    echo "(none)"
  else
    while IFS= read -r d; do
      echo "$d ($(wc -l <"$d") lines)"
      grep -E '^#{1,2} ' "$d" | sed 's/^/    /' || true
    done <<<"$docs"
  fi
  echo

  echo "## Changes since last docs sync"
  if [ ! -f "$MANIFEST" ]; then
    echo "NO MANIFEST - docs have never been synced by this skill"
  else
    echo "last sync: $(sed -n 's/^# synced_at: //p' "$MANIFEST")  commit: $(sed -n 's/^# commit: //p' "$MANIFEST")"
    grep -v '^#' "$MANIFEST" >"$TMP/old" || true
    source_files | while IFS= read -r f; do hash_file "$f"; done >"$TMP/new"
    awk 'NR==FNR { old[substr($0,67)] = substr($0,1,64); next }
         { p = substr($0,67); h = substr($0,1,64); seen[p] = 1
           if (!(p in old)) print "NEW      " p; else if (old[p] != h) print "CHANGED  " p }
         END { for (p in old) if (!(p in seen)) print "DELETED  " p }' "$TMP/old" "$TMP/new" | LC_ALL=C sort -k2 >"$TMP/delta"
    if [ -s "$TMP/delta" ]; then cat "$TMP/delta"; else echo "(no source changes)"; fi
  fi
  echo

  echo "## Go source files (non-test): lines, header comment, top-level symbols"
  go_files | while IFS= read -r f; do
    perl -e '
      my $f = shift; open my $fh, "<", $f or exit; my @L = <$fh>; close $fh;
      my ($c, @types, @funcs) = ("");
      for my $i (0 .. ($#L < 40 ? $#L : 40)) {
        last if $c ne "" && $L[$i] !~ m{^\s*//};
        next if $L[$i] =~ m{^//go:|^// \+build};
        if ($L[$i] =~ m{^\s*//\s?(.*)}) { $c .= ($c eq "" ? "" : " ") . $1 }
      }
      for (@L) {
        push @types, $1 if /^type\s+(\w+)/;
        push @funcs, ($1 ? "$1." : "") . $2 if /^func\s+(?:\(\s*\w*\s*\*?(\w+)[^)]*\)\s*)?(\w+)/;
      }
      $c = substr($c, 0, 160) . "..." if length $c > 160;
      my $fl = @funcs > 12 ? join(", ", @funcs[0..11]) . sprintf(" (+%d more)", @funcs - 12) : join(", ", @funcs);
      printf "%s (%d lines)%s\n", $f, scalar @L, $c ne "" ? " - $c" : "";
      printf "    types: %s\n", join(", ", @types) if @types;
      printf "    funcs: %s\n", $fl if @funcs;
    ' "$f"
  done
  echo

  echo "## Environment variables (name | default | where read)"
  echo "   <redacted> = secret-looking name with a hardcoded default: document the name, never the value"
  env_vars | awk -F'\t' '{ printf "%-24s %-50s %s\n", $1, $2, $3 }'
  echo

  echo "## HTTP routes (path | access | where registered)"
  routes | awk -F'\t' '{ printf "%-34s %-12s %s\n", $1, $2, $3 }'
  echo

  echo "## Network listeners / outbound dials"
  go_files | while IFS= read -r f; do
    grep -nE 'net\.Listen|ListenAndServe|ListenAndServeTLS|NewAcceptor|NewInitiator|websocket\.(Dialer|Upgrader)|\.Dial\(|http\.(Get|Post|NewRequest)' "$f" \
      | sed "s|^|$f:|" | sed -E 's/[[:space:]]+/ /g' | cut -c1-160 || true
  done
  echo

  echo "## File paths referenced in code"
  go_files | while IFS= read -r f; do
    grep -noE '"(/(opt|etc|var|usr|home|tmp|srv)/[^"]*|[A-Za-z0-9_./-]+\.(json|jsonl|tsv|csv|log|xml|env|txt|cfg|pem|crt|key))"' "$f" \
      | sed "s|^|$f:|" || true
  done
  echo

  echo "## Data, config, ops and spec files in repo"
  source_files | grep -vE '\.(go|mod)$' | while IFS= read -r f; do
    note=""
    case "$f" in
      *.sh) note=$(sed -n '2,6p' "$f" | grep -E '^#' | sed 's/^# \{0,1\}//' | tr '\n' ' ' | cut -c1-140) ;;
      # Shape only - these files can hold credentials, so never print keys or values.
      *.json) note=$(perl -MJSON::PP -e 'local $/; my $j = eval { JSON::PP->new->decode(<STDIN>) };
                       sub shape { my $v = shift; ref $v eq "HASH" ? "object{" . join(",", sort keys %$v) . "}" : ref $v eq "ARRAY" ? "array" : "scalar" }
                       print ref $j eq "HASH"  ? "map of " . scalar(keys %$j) . " entries; values: " . shape((values %$j)[0])
                           : ref $j eq "ARRAY" ? "array of " . scalar(@$j) . "; items: " . shape($j->[0])
                           : "not valid JSON"' <"$f" 2>/dev/null || true) ;;
      *.tsv) note="$(head -1 "$f" | awk -F'\t' '{print NF}') tab-separated columns, $(wc -l <"$f" | tr -d ' ') lines" ;;
    esac
    printf '%s (%s bytes)%s\n' "$f" "$(wc -c <"$f" | tr -d ' ')" "${note:+ - $note}"
  done
  echo

  echo "## Tests"
  source_files | grep -E '_test\.go$' | while IFS= read -r f; do
    printf '%s: %s\n' "$f" "$(grep -oE '^func (Test|Benchmark|Example|Fuzz)\w+' "$f" | sed 's/^func //' | tr '\n' ' ')"
  done || true
}

check_docs() {
  local docs; docs=$(doc_files)
  if [ -z "$docs" ]; then echo "RESULT: FAIL - no README.md or docs/*.md"; exit 1; fi
  while IFS= read -r d; do cat "$d"; echo; done <<<"$docs" >"$TMP/corpus"
  env_vars | cut -f1 >"$TMP/envs"
  routes | cut -f1 | LC_ALL=C sort -u >"$TMP/routes"

  perl -e '
    my ($corpus, $envs, $routes) = @ARGV;
    local $/; open my $c, "<", $corpus or die; my $doc = <$c>; close $c; $/ = "\n";
    open my $e, "<", $envs or die;   chomp(my @env = <$e>);   close $e;
    open my $r, "<", $routes or die; chomp(my @rt = <$r>);    close $r;
    my %env = map { $_ => 1 } @env; my %rt = map { $_ => 1 } @rt;
    my ($miss, $stale) = (0, 0);
    for my $n (@env) { unless ($doc =~ /(?<![A-Za-z0-9_])\Q$n\E(?![A-Za-z0-9_])/) { print "MISSING env var:      $n\n"; $miss++ } }
    for my $p (@rt) {
      my $ok = $p eq "/" ? index($doc, "`/`") >= 0 || $doc =~ /\|\s*`?(?:[A-Z]+ )?\/`?\s*\|/
                         : $doc =~ /(?<![\w\/.-])\Q$p\E(?![\w\/-])/;
      unless ($ok) { print "MISSING route:        $p\n"; $miss++ }
    }
    my %rep;
    while ($doc =~ /`([A-Z][A-Z0-9]*_[A-Z0-9_]+)`/g) {
      next if $env{$1} || $rep{$1}++; print "POSSIBLY STALE env:   $1 (in docs, not read by code)\n"; $stale++;
    }
    while ($doc =~ /`(?:(?:GET|POST|PUT|PATCH|DELETE|HEAD) )?(\/[A-Za-z0-9_\/.{}:-]*)(?:\?[^`]*)?`/g) {
      my $p = $1;
      next if $p =~ m{^/(opt|etc|var|usr|home|tmp|srv|dev|proc)(/|$)} || $p =~ /\.\w+$/;
      next if $rt{$p} || $rep{$p}++;
      print "POSSIBLY STALE route: $p (in docs, not registered in code)\n"; $stale++;
    }
    printf "RESULT: %s - %d missing, %d possibly stale (review stale items; they may be legitimate)\n",
      $miss ? "FAIL" : "PASS", $miss, $stale;
    exit($miss ? 1 : 0);
  ' "$TMP/corpus" "$TMP/envs" "$TMP/routes"
}

write_manifest() {
  mkdir -p "$DOCS_DIR"
  {
    echo "# update-docs sync manifest - written by .claude/skills/update-docs; do not edit by hand"
    echo "# synced_at: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "# commit: $(git rev-parse HEAD 2>/dev/null || echo none)"
    source_files | while IFS= read -r f; do hash_file "$f"; done
  } >"$MANIFEST"
  echo "wrote $MANIFEST ($(grep -vc '^#' "$MANIFEST") source files recorded)"
}

case "${1:-}" in
  "")               print_inventory ;;
  --check)          check_docs ;;
  --write-manifest) write_manifest ;;
  *) echo "usage: $0 [--check | --write-manifest]" >&2; exit 2 ;;
esac
