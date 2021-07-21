#!/usr/bin/env bash

. "$(dirname "$0")/testlib.sh"

reponame="$(basename "$0" ".sh")"
contents="a"
contents_oid=$(calc_oid "$contents")

begin_test "init fetch unclean paths"
(
  set -e

  setup_remote_repo $reponame
  clone_repo $reponame repo

  git lfs track "*.dat" 2>&1 | tee track.log
  grep "Tracking \"\*.dat\"" track.log

  mkdir dir
  printf "%s" "$contents" > dir/a.dat

  git add dir/a.dat
  git add .gitattributes
  git commit -m "add dir/a.dat" 2>&1 | tee commit.log
  grep "main (root-commit)" commit.log
  grep "2 files changed" commit.log
  grep "create mode 100644 dir/a.dat" commit.log
  grep "create mode 100644 .gitattributes" commit.log

  [ "a" = "$(cat dir/a.dat)" ]

  assert_local_object "$contents_oid" 1
  refute_server_object "$contents_oid"

  git push origin main 2>&1 | tee push.log
  grep "Uploading LFS objects: 100% (1/1), 1 B" push.log
  grep "main -> main" push.log

  assert_server_object "$reponame" "$contents_oid"

  # This clone is used for subsequent tests
  clone_repo "$reponame" clone
)
end_test

begin_test "fetch unclean paths with include filter in gitconfig"
(
  set -e
  cd clone
  rm -rf .git/lfs/objects

  git config "lfs.fetchinclude" "dir/**"
  git lfs fetch
  assert_local_object "$contents_oid" 1
)
end_test

begin_test "fetch unclean paths with exclude filter in gitconfig"
(
  set -e
  cd clone
  rm -rf .git/lfs/objects
  git config --unset "lfs.fetchinclude"

  git config "lfs.fetchexclude" "dir/**"
  git lfs fetch
  refute_local_object "$contents_oid"
)
end_test

begin_test "fetch unclean paths with include filter in cli"
(
  set -e
  cd clone
  rm -rf .git/lfs/objects

  git config --unset "lfs.fetchexclude"

  rm -rf .git/lfs/objects
  git lfs fetch -I="dir/**"
  assert_local_object "$contents_oid" 1
)
end_test

begin_test "fetch unclean paths with exclude filter in cli"
(
  set -e
  cd clone
  rm -rf .git/lfs/objects

  git lfs fetch -X="dir/**"
  refute_local_object "$contents_oid"
)
end_test
