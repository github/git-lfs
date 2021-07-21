#!/usr/bin/env bash

. "$(dirname "$0")/testlib.sh"

begin_test "ls-files"
(
  set -e

  mkdir repo
  cd repo
  git init
  git lfs track "*.dat" | grep "Tracking \"\*.dat\""
  echo "some data" > some.dat
  echo "some text" > some.txt
  echo "missing" > missing.dat
  git add missing.dat
  git commit -m "add missing file"
  [ "6bbd052ab0 * missing.dat" = "$(git lfs ls-files)" ]

  git rm missing.dat
  git add some.dat some.txt
  git commit -m "added some files, removed missing one"

  git lfs ls-files | tee ls.log
  grep some.dat ls.log
  [ `wc -l < ls.log` = 1 ]

  diff -u <(git lfs ls-files --debug) <(cat <<-EOF
filepath: some.dat
    size: 10
checkout: true
download: true
     oid: sha256 5aa03f96c77536579166fba147929626cc3a97960e994057a9d80271a736d10f
 version: https://git-lfs.github.com/spec/v1

EOF)
)
end_test

begin_test "ls-files: --size"
(
  set -e

  reponame="ls-files-size"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.dat"
  git add .gitattributes
  git commit -m "initial commit"

  contents="contents"
  size="$(printf "%s" "$contents" | wc -c | awk '{ print $1 }')"
  printf "%s" "$contents" > a.dat

  git add a.dat
  git commit -m "add a.dat"

  git lfs ls-files --size 2>&1 | tee ls.log
  [ "d1b2a59fbe * a.dat (8 B)" = "$(cat ls.log)" ]
)
end_test

begin_test "ls-files: indexed files without tree"
(
  set -e

  reponame="ls-files-indexed-files-without-tree"
  git init "$reponame"
  cd "$reponame"

  git lfs track '*.dat'
  git add .gitattributes

  contents="a"
  oid="$(calc_oid "$contents")"
  printf "%s" "$contents" > a.dat

  [ "" = "$(git lfs ls-files)" ]

  git add a.dat

  [ "${oid:0:10} * a.dat" = "$(git lfs ls-files)" ]
)
end_test

begin_test "ls-files: indexed file with tree"
(
  set -e

  reponame="ls-files-indexed-files-with-tree"
  git init "$reponame"
  cd "$reponame"

  git lfs track '*.dat'
  git add .gitattributes
  git commit -m "initial commit"

  tree_contents="a"
  tree_oid="$(calc_oid "$tree_contents")"

  printf "%s" "$tree_contents" > a.dat
  git add a.dat
  git commit -m "add a.dat"

  index_contents="b"
  index_oid="$(calc_oid "$index_contents")"

  printf "%s" "$index_contents" > a.dat
  git add a.dat

  [ "${index_oid:0:10} * a.dat" = "$(git lfs ls-files)" ]
)
end_test

begin_test "ls-files: historical reference ignores index"
(
  set -e

  reponame="ls-files-historical-reference-ignores-index"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.txt"
  echo "a.txt" > a.txt
  echo "b.txt" > b.txt
  echo "c.txt" > c.txt

  git add .gitattributes a.txt
  git commit -m "a.txt: initial commit"

  git add b.txt
  git commit -m "b.txt: initial commit"

  git add c.txt

  git lfs ls-files "$(git rev-parse HEAD~1)" 2>&1 | tee ls-files.log

  [ 1 -eq "$(grep -c "a.txt" ls-files.log)" ]
  [ 0 -eq "$(grep -c "b.txt" ls-files.log)" ]
  [ 0 -eq "$(grep -c "c.txt" ls-files.log)" ]
)
end_test

begin_test "ls-files: non-HEAD reference referring to HEAD ignores index"
(
  set -e

  reponame="ls-files-HEAD-ish-ignores-index"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.txt"
  echo "a.txt" > a.txt
  echo "b.txt" > b.txt

  git add .gitattributes a.txt
  git commit -m "a.txt: initial commit"

  tagname="v1.0.0"
  git tag "$tagname"

  git add b.txt

  git lfs ls-files "$tagname" 2>&1 | tee ls-files.log

  [ 1 -eq "$(grep -c "a.txt" ls-files.log)" ]
  [ 0 -eq "$(grep -c "b.txt" ls-files.log)" ]
)
end_test

begin_test "ls-files: outside git repository"
(
  set +e
  git lfs ls-files 2>&1 > ls-files.log
  res=$?

  set -e
  if [ "$res" = "0" ]; then
    echo "Passes because $GIT_LFS_TEST_DIR is unset."
    exit 0
  fi
  [ "$res" = "128" ]
  grep "Not in a git repository" ls-files.log
)
end_test

begin_test "ls-files: --include"
(
  set -e

  git init ls-files-include
  cd ls-files-include

  git lfs track "*.dat" "*.bin"
  echo "a" > a.dat
  echo "b" > b.dat
  echo "c" > c.bin

  git add *.gitattributes a.dat b.dat c.bin
  git commit -m "initial commit"

  git lfs ls-files --include="*.dat" 2>&1 | tee ls-files.log

  [ "0" -eq "$(grep -c "\.bin" ls-files.log)" ]
  [ "2" -eq "$(grep -c "\.dat" ls-files.log)" ]
)
end_test

begin_test "ls-files: --exclude"
(
  set -e

  git init ls-files-exclude
  cd ls-files-exclude

  mkdir dir

  git lfs track "*.dat"
  echo "a" > a.dat
  echo "b" > b.dat
  echo "c" > dir/c.dat

  git add *.gitattributes a.dat b.dat dir/c.dat
  git commit -m "initial commit"

  git lfs ls-files --exclude="dir/**" 2>&1 | tee ls-files.log

  [ "0" -eq "$(grep -c "dir" ls-files.log)" ]
  [ "2" -eq "$(grep -c "\.dat" ls-files.log)" ]
)
end_test

begin_test "ls-files: before first commit"
(
  set -e

  reponame="ls-files-before-first-commit"
  git init "$reponame"
  cd "$reponame"

  if [ 0 -ne $(git lfs ls-files | wc -l) ]; then
    echo >&2 "fatal: expected \`git lfs ls-files\` to produce no output"
    exit 1
  fi
)
end_test

begin_test "ls-files: show duplicate files"
(
  set -e

  mkdir dupRepoShort
  cd dupRepoShort
  git init

  git lfs track "*.tgz" | grep "Tracking \"\*.tgz\""
  echo "test content" > one.tgz
  echo "test content" > two.tgz
  git add one.tgz
  git add two.tgz
  git commit -m "add duplicate files"

  expected="$(echo "a1fff0ffef * one.tgz
a1fff0ffef * two.tgz")"

  [ "$expected" = "$(git lfs ls-files)" ]
)
end_test

begin_test "ls-files: show duplicate files with long OID"
(
  set -e

  mkdir dupRepoLong
  cd dupRepoLong
  git init

  git lfs track "*.tgz" | grep "Tracking \"\*.tgz\""
  echo "test content" > one.tgz
  echo "test content" > two.tgz
  git add one.tgz
  git add two.tgz
  git commit -m "add duplicate files with long OID"

  expected="$(echo "a1fff0ffefb9eace7230c24e50731f0a91c62f9cefdfe77121c2f607125dffae * one.tgz
a1fff0ffefb9eace7230c24e50731f0a91c62f9cefdfe77121c2f607125dffae * two.tgz")"

  [ "$expected" = "$(git lfs ls-files --long)" ]
)
end_test

begin_test "ls-files: history with --all"
(
  set -e

  reponame="ls-files-history-with-all"
  git init "$reponame"
  cd "$reponame"

  git lfs track '*.dat'
  printf "a" > a.dat
  printf "b" > b.dat

  git add .gitattributes a.dat b.dat
  git commit -m "initial commit"

  rm b.dat
  git add b.dat
  git commit -m "remove b.dat"

  git lfs ls-files 2>&1 | tee ls-files.log
  [ 1 -eq $(grep -c "a\.dat" ls-files.log) ]
  [ 0 -eq $(grep -c "b\.dat" ls-files.log) ]

  git lfs ls-files --all 2>&1 | tee ls-files-all.log
  [ 1 -eq $(grep -c "a\.dat" ls-files-all.log) ]
  [ 1 -eq $(grep -c "b\.dat" ls-files-all.log) ]
)
end_test

begin_test "ls-files: --all with argument(s)"
(
  set -e

  reponame="ls-files-all-with-arguments"
  git init "$reponame"
  cd "$reponame"

  git lfs ls-files --all main 2>&1 | tee ls-files.log

  if [ "0" -eq "${PIPESTATUS[0]}" ]; then
    echo >&2 "fatal: \`git lfs ls-files --all main\` to fail"
    exit 1
  fi

  [ "fatal: cannot use --all with explicit reference" = "$(cat ls-files.log)" ]
)
end_test

begin_test "ls-files: reference with --deleted"
(
  set -e

  reponame="ls-files-reference-with-deleted"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.dat"
  printf "a" > a.dat
  git add .gitattributes a.dat
  git commit -m "initial commit"

  rm a.dat
  git add a.dat
  git commit -m "a.dat: remove a.dat"

  git lfs ls-files 2>&1 | tee ls-files.log
  git lfs ls-files --deleted 2>&1 | tee ls-files-deleted.log

  [ 0 -eq $(grep -c "a\.dat" ls-files.log) ]
  [ 1 -eq $(grep -c "a\.dat" ls-files-deleted.log) ]
)
end_test

begin_test "ls-files: invalid --all ordering"
(
  set -e

  reponame="ls-files-invalid---all-ordering"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.dat"
  echo "Hello world" > a.dat

  git add .gitattributes a.dat
  git commit -m "initial commit"

  git lfs ls-files -- --all 2>&1 | tee ls-files.out
  if [ ${PIPESTATUS[0]} = "0" ]; then
    echo >&2 "fatal: expected \`git lfs ls-files -- --all\' to fail"
    exit 1
  fi
  grep "fatal: did you mean \"git lfs ls-files --all --\" ?" ls-files.out
)
end_test

begin_test "ls-files: list/stat files with escaped runes in path before commit"
(
  set -e

  reponame=runes-in-path
  content="zero"
  checksum="d3eb539a55"
  pathWithGermanRunes="german/äöü"
  fileWithGermanRunes="schüüch.bin"

  mkdir $reponame
  git init "$reponame"
  cd $reponame
  git lfs track "**/*"

  echo "$content" > regular
  echo "$content" > "$fileWithGermanRunes"

  mkdir -p "$pathWithGermanRunes"
  echo "$content" > "$pathWithGermanRunes/regular"
  echo "$content" > "$pathWithGermanRunes/$fileWithGermanRunes"

  git add *

  # check short form
  [ 4 -eq "$(git lfs ls-files | grep -c '*')" ]

  # also check long format
  [ 4 -eq "$(git lfs ls-files -l | grep -c '*')" ]

)
end_test

begin_test "ls-files: --name-only"
(
  set -e

  reponame="ls-files-name"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.dat"
  git add .gitattributes
  git commit -m "initial commit"

  contents="test contents"
  echo "$contents" > a.dat

  git add a.dat
  git commit -m "add a.dat"

  git lfs ls-files --name-only 2>&1 | tee ls.log
  [ "a.dat" = "$(cat ls.log)" ]
)
end_test

begin_test "ls-files: history with reference range"
(
  set -e

  reponame="ls-files-history-with-range"
  git init "$reponame"
  cd "$reponame"

  git lfs track "*.dat"
  git add .gitattributes
  git commit -m 'intial commit'

  echo "content of a-file" > a.dat
  git add a.dat
  git commit -m 'add a.dat'

  echo "content of b-file" > b.dat
  git add b.dat
  git commit -m 'add b.dat'

  git tag b-commit

  echo "content of c-file" > c.dat
  git add c.dat
  git commit -m 'add c.dat'

  echo "content of c-file and later modified" > c.dat
  git add c.dat
  git commit -m 'modify c.dat'

  git tag c-commit

  git rm a.dat
  git commit -m 'remove a.dat'

  git lfs ls-files --all 2>&1 | tee ls-files.log
  [ 1 -eq $(grep -c "a\.dat" ls-files.log) ]
  [ 1 -eq $(grep -c "b\.dat" ls-files.log) ]
  [ 2 -eq $(grep -c "c\.dat" ls-files.log) ]

  git lfs ls-files b-commit c-commit 2>&1 | tee ls-files.log
  [ 0 -eq $(grep -c "a\.dat" ls-files.log) ]
  [ 0 -eq $(grep -c "b\.dat" ls-files.log) ]
  [ 2 -eq $(grep -c "c\.dat" ls-files.log) ]

  git lfs ls-files c-commit~ c-commit 2>&1 | tee ls-files.log
  [ 0 -eq $(grep -c "a\.dat" ls-files.log) ]
  [ 0 -eq $(grep -c "b\.dat" ls-files.log) ]
  [ 1 -eq $(grep -c "c\.dat" ls-files.log) ]

  git lfs ls-files HEAD~ HEAD 2>&1 | tee ls-files.log
  [ 0 -eq $(grep -c "a\.dat" ls-files.log) ]
  [ 0 -eq $(grep -c "b\.dat" ls-files.log) ]
  [ 0 -eq $(grep -c "c\.dat" ls-files.log) ]
)
end_test

begin_test "ls-files: not affected by lfs.fetchexclude"
(
  set -e

  mkdir repo-fetchexclude
  cd repo-fetchexclude
  git init
  git lfs track "*.dat" | grep "Tracking \"\*.dat\""
  echo "some data" > some.dat
  echo "some text" > some.txt
  echo "missing" > missing.dat
  git add missing.dat
  git commit -m "add missing file"
  git config lfs.fetchexclude '*'
  [ "6bbd052ab0 * missing.dat" = "$(git lfs ls-files)" ]
)
end_test