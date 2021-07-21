#!/usr/bin/env bash

. "$(dirname "$0")/fixtures/migrate.sh"
. "$(dirname "$0")/testlib.sh"

begin_test "migrate import (default branch)"
(
  set -e

  setup_multiple_local_branches

  md_oid="$(calc_oid "$(git cat-file -p :a.md)")"
  txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  md_feature_oid="$(calc_oid "$(git cat-file -p my-feature:a.md)")"

  git lfs migrate import

  assert_pointer "refs/heads/main" "a.md" "$md_oid" "140"
  assert_pointer "refs/heads/main" "a.txt" "$txt_oid" "120"

  assert_local_object "$md_oid" "140"
  assert_local_object "$txt_oid" "120"
  refute_local_object "$md_feature_oid" "30"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  [ ! $(git cat-file -p "$feature:.gitattributes") ]

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"

  # Ensure that hooks are installed. If we find 'git-lfs' somewhere in
  # .git/hooks/pre-push we assume that the rest went correctly, too.
  grep -q "git-lfs" .git/hooks/pre-push
)
end_test

begin_test "migrate import (bare repository)"
(
  set -e

  setup_multiple_remote_branches

  git lfs migrate import --everything
)
end_test

begin_test "migrate import (given branch)"
(
  set -e

  setup_multiple_local_branches

  md_oid="$(calc_oid "$(git cat-file -p :a.md)")"
  txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  md_feature_oid="$(calc_oid "$(git cat-file -p my-feature:a.md)")"

  git lfs migrate import my-feature

  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "30"
  assert_pointer "refs/heads/my-feature" "a.txt" "$txt_oid" "120"
  assert_pointer "refs/heads/main" "a.md" "$md_oid" "140"
  assert_pointer "refs/heads/main" "a.txt" "$txt_oid" "120"

  assert_local_object "$md_oid" "140"
  assert_local_object "$md_feature_oid" "30"
  assert_local_object "$txt_oid" "120"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  feature_attrs="$(git cat-file -p "$feature:.gitattributes")"

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (default branch with filter)"
(
  set -e

  setup_multiple_local_branches

  md_oid="$(calc_oid "$(git cat-file -p :a.md)")"
  txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  md_feature_oid="$(calc_oid "$(git cat-file -p my-feature:a.md)")"

  git lfs migrate import --include "*.md"

  assert_pointer "refs/heads/main" "a.md" "$md_oid" "140"

  assert_local_object "$md_oid" "140"
  refute_local_object "$txt_oid" "120"
  refute_local_object "$md_feature_oid" "30"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  [ ! $(git cat-file -p "$feature:.gitattributes") ]

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -vq "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (given branch with filter)"
(
  set -e

  setup_multiple_local_branches

  md_oid="$(calc_oid "$(git cat-file -p :a.md)")"
  txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  md_feature_oid="$(calc_oid "$(git cat-file -p my-feature:a.md)")"

  git lfs migrate import --include "*.md" my-feature

  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "30"
  assert_pointer "refs/heads/my-feature~1" "a.md" "$md_oid" "140"

  assert_local_object "$md_oid" "140"
  assert_local_object "$md_feature_oid" "30"
  refute_local_object "$txt_oid" "120"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  feature_attrs="$(git cat-file -p "$feature:.gitattributes")"

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -vq "*.txt filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -vq "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (default branch, exclude remote refs)"
(
  set -e

  setup_single_remote_branch

  md_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.md")")"
  txt_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.txt")")"
  md_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  txt_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"

  git lfs migrate import

  assert_pointer "refs/heads/main" "a.md" "$md_oid" "50"
  assert_pointer "refs/heads/main" "a.txt" "$txt_oid" "30"

  assert_local_object "$md_oid" "50"
  assert_local_object "$txt_oid" "30"
  refute_local_object "$md_remote_oid" "140"
  refute_local_object "$txt_remote_oid" "120"

  main="$(git rev-parse refs/heads/main)"
  remote="$(git rev-parse refs/remotes/origin/main)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  [ ! $(git cat-file -p "$remote:.gitattributes") ]

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -vq "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (given branch, exclude remote refs)"
(
  set -e

  setup_multiple_remote_branches

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  md_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.md")")"
  md_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  txt_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.txt")")"
  txt_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.txt")")"

  git lfs migrate import my-feature

  assert_pointer "refs/heads/main" "a.md" "$md_main_oid" "21"
  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "31"
  assert_pointer "refs/heads/main" "a.txt" "$txt_main_oid" "20"
  assert_pointer "refs/heads/my-feature" "a.txt" "$txt_feature_oid" "30"

  assert_local_object "$md_feature_oid" "31"
  assert_local_object "$md_main_oid" "21"
  assert_local_object "$txt_feature_oid" "30"
  assert_local_object "$txt_main_oid" "20"
  refute_local_object "$md_remote_oid" "11"
  refute_local_object "$txt_remote_oid" "10"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"
  remote="$(git rev-parse refs/remotes/origin/main)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  [ ! $(git cat-file -p "$remote:.gitattributes") ]
  feature_attrs="$(git cat-file -p "$feature:.gitattributes")"

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -vq "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (given ref, --skip-fetch)"
(
  set -e

  setup_single_remote_branch

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  md_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  txt_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.txt")")"

  git tag pseudo-remote "$(git rev-parse refs/remotes/origin/main)"
  # Remove the refs/remotes/origin/main ref, and instruct 'git lfs migrate' to
  # not fetch it.
  git update-ref -d refs/remotes/origin/main

  git lfs migrate import --skip-fetch

  assert_pointer "refs/heads/main" "a.md" "$md_main_oid" "50"
  assert_pointer "pseudo-remote" "a.md" "$md_remote_oid" "140"
  assert_pointer "refs/heads/main" "a.txt" "$txt_main_oid" "30"
  assert_pointer "pseudo-remote" "a.txt" "$txt_remote_oid" "120"

  assert_local_object "$md_main_oid" "50"
  assert_local_object "$txt_main_oid" "30"
  assert_local_object "$md_remote_oid" "140"
  assert_local_object "$txt_remote_oid" "120"

  main="$(git rev-parse refs/heads/main)"
  remote="$(git rev-parse pseudo-remote)"

  main_attrs="$(git cat-file -p "$main:.gitattributes")"
  remote_attrs="$(git cat-file -p "$remote:.gitattributes")"

  echo "$main_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
  echo "$remote_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$remote_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (un-annotated tags)"
(
  set -e

  setup_single_local_branch_with_tags

  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"

  git lfs migrate import --everything

  assert_pointer "refs/heads/main" "a.txt" "$txt_main_oid" "2"
  assert_local_object "$txt_main_oid" "2"

  git tag --points-at "$(git rev-parse HEAD)" | grep -q "v1.0.0"
)
end_test

begin_test "migrate import (annotated tags)"
(
  set -e

  setup_single_local_branch_with_annotated_tags

  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"

  git lfs migrate import --everything

  assert_pointer "refs/heads/main" "a.txt" "$txt_main_oid" "2"
  assert_local_object "$txt_main_oid" "2"

  git tag --points-at "$(git rev-parse HEAD)" | grep -q "v1.0.0"
)
end_test

begin_test "migrate import (include/exclude ref)"
(
  set -e

  setup_multiple_remote_branches

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  md_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.md")")"
  md_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  txt_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.txt")")"
  txt_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.txt")")"

  git lfs migrate import \
    --include-ref=refs/heads/my-feature \
    --exclude-ref=refs/heads/main

  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "31"
  assert_pointer "refs/heads/my-feature" "a.txt" "$txt_feature_oid" "30"

  assert_local_object "$md_feature_oid" "31"
  refute_local_object "$md_main_oid" "21"
  assert_local_object "$txt_feature_oid" "30"
  refute_local_object "$txt_main_oid" "20"
  refute_local_object "$md_remote_oid" "11"
  refute_local_object "$txt_remote_oid" "10"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"
  remote="$(git rev-parse refs/remotes/origin/main)"

  [ ! $(git cat-file -p "$main:.gitattributes") ]
  [ ! $(git cat-file -p "$remote:.gitattributes") ]
  feature_attrs="$(git cat-file -p "$feature:.gitattributes")"

  echo "$feature_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (include/exclude ref args)"
(
  set -e

  setup_multiple_remote_branches

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  md_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.md")")"
  md_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  txt_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.txt")")"
  txt_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.txt")")"

  git lfs migrate import my-feature ^main

  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "31"
  assert_pointer "refs/heads/my-feature" "a.txt" "$txt_feature_oid" "30"

  assert_local_object "$md_feature_oid" "31"
  refute_local_object "$md_main_oid" "21"
  assert_local_object "$txt_feature_oid" "30"
  refute_local_object "$txt_main_oid" "20"
  refute_local_object "$md_remote_oid" "11"
  refute_local_object "$txt_remote_oid" "10"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"
  remote="$(git rev-parse refs/remotes/origin/main)"

  [ ! $(git cat-file -p "$main:.gitattributes") ]
  [ ! $(git cat-file -p "$remote:.gitattributes") ]
  feature_attrs="$(git cat-file -p "$feature:.gitattributes")"

  echo "$feature_attrs" | grep -q "*.md filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (include/exclude ref with filter)"
(
  set -e

  setup_multiple_remote_branches

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  md_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.md")")"
  md_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  txt_remote_oid="$(calc_oid "$(git cat-file -p "refs/remotes/origin/main:a.txt")")"
  txt_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.txt")")"

  git lfs migrate import \
    --include="*.txt" \
    --include-ref=refs/heads/my-feature \
    --exclude-ref=refs/heads/main

  assert_pointer "refs/heads/my-feature" "a.txt" "$txt_feature_oid" "30"

  refute_local_object "$md_feature_oid" "31"
  refute_local_object "$md_main_oid" "21"
  assert_local_object "$txt_feature_oid" "30"
  refute_local_object "$txt_main_oid" "20"
  refute_local_object "$md_remote_oid" "11"
  refute_local_object "$txt_remote_oid" "10"

  main="$(git rev-parse refs/heads/main)"
  feature="$(git rev-parse refs/heads/my-feature)"
  remote="$(git rev-parse refs/remotes/origin/main)"

  [ ! $(git cat-file -p "$main:.gitattributes") ]
  [ ! $(git cat-file -p "$remote:.gitattributes") ]
  feature_attrs="$(git cat-file -p "$feature:.gitattributes")"

  echo "$feature_attrs" | grep -vq "*.md filter=lfs diff=lfs merge=lfs"
  echo "$feature_attrs" | grep -q "*.txt filter=lfs diff=lfs merge=lfs"
)
end_test

begin_test "migrate import (above)"
(
  set -e
  setup_single_local_branch_untracked

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"

  git lfs migrate import --above 121B
  # Ensure that 'a.md', whose size is above our 121 byte threshold
  # was converted into a git-lfs pointer by the migration.
  assert_local_object "$md_main_oid" "140"
  assert_pointer "refs/heads/main" "a.md" "$md_main_oid" "140"
  refute_pointer "refs/heads/main" "a.txt" "$txt_main_oid" "120"
  refute_local_object "$txt_main_oid" "120"

  # The migration should have identified that *.md files are now
  # tracked because it migrated a.md
  main_attrs="$(git cat-file -p "$main:.gitattributes")"

  echo "$main_attrs" | grep -q "/a.md filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -vq "/a.txt filter=lfs diff=lfs merge=lfs"
  git check-attr filter -- a.txt | grep -vq lfs
)
end_test

begin_test "migrate import (above without extension)"
(
  set -e
  setup_single_local_branch_untracked "just-b"

  b_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:just-b")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"

  git lfs migrate import --above 121B
  # Ensure that 'b', whose size is above our 121 byte threshold
  # was converted into a git-lfs pointer by the migration.
  assert_local_object "$b_main_oid" "140"
  assert_pointer "refs/heads/main" "just-b" "$b_main_oid" "140"
  refute_pointer "refs/heads/main" "a.txt" "$txt_main_oid" "120"
  refute_local_object "$txt_main_oid" "120"

  # The migration should have identified that /b is now tracked
  # because it migrated it.
  main_attrs="$(git cat-file -p "$main:.gitattributes")"

  echo "$main_attrs" | grep -q "/just-b filter=lfs diff=lfs merge=lfs"
  echo "$main_attrs" | grep -vq "/a.txt filter=lfs diff=lfs merge=lfs"
  git check-attr filter -- a.txt | grep -vq lfs
)
end_test

begin_test "migrate import (above with multiple files)"
(
  set -e
  # It is important that this file sort after "a.txt".
  setup_single_local_branch_untracked "b.txt"

  a_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  b_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:b.txt")")"

  git lfs migrate import --above 121B
  # Ensure that 'a.md', whose size is above our 121 byte threshold
  # was converted into a git-lfs pointer by the migration.
  assert_local_object "$b_main_oid" "140"
  assert_pointer "refs/heads/main" "b.txt" "$b_main_oid" "140"
  refute_pointer "refs/heads/main" "a.txt" "$a_main_oid" "120"
  refute_local_object "$a_main_oid" "120"

  # The migration should have identified that *.md files are now
  # tracked because it migrated a.md
  main_attrs="$(git cat-file -p "$main:.gitattributes")"

  echo "$main_attrs" | grep -q "/b.txt filter=lfs diff=lfs merge=lfs"
  git check-attr filter -- a.txt | grep -vq lfs
)
end_test

begin_test "migrate import (existing .gitattributes)"
(
  set -e

  setup_local_branch_with_gitattrs

  main="$(git rev-parse refs/heads/main)"

  txt_main_oid="$(calc_oid "$(git cat-file -p "$main:a.txt")")"

  git lfs migrate import --yes --include-ref=refs/heads/main --include="*.txt"

  assert_local_object "$txt_main_oid" "120"

  main="$(git rev-parse refs/heads/main)"
  prev="$(git rev-parse refs/heads/main^1)"

  diff -u <(git cat-file -p $main:.gitattributes) <(cat <<-EOF
*.txt filter=lfs diff=lfs merge=lfs -text
*.other filter=lfs diff=lfs merge=lfs -text
EOF)

  diff -u <(git cat-file -p $prev:.gitattributes) <(cat <<-EOF
*.txt filter=lfs diff=lfs merge=lfs -text
EOF)
)
end_test

begin_test "migrate import (--exclude with existing .gitattributes)"
(
  set -e

  setup_local_branch_with_gitattrs

  main="$(git rev-parse refs/heads/main)"

  txt_main_oid="$(calc_oid "$(git cat-file -p "$main:a.txt")")"

  git lfs migrate import --yes --include-ref=refs/heads/main --include="*.txt" --exclude="*.bin"

  assert_local_object "$txt_main_oid" "120"

  main="$(git rev-parse refs/heads/main)"
  prev="$(git rev-parse refs/heads/main^1)"

  diff -u <(git cat-file -p $main:.gitattributes) <(cat <<-EOF
*.txt filter=lfs diff=lfs merge=lfs -text
*.other filter=lfs diff=lfs merge=lfs -text
*.bin !text -filter -merge -diff
EOF)

  diff -u <(git cat-file -p $prev:.gitattributes) <(cat <<-EOF
*.txt filter=lfs diff=lfs merge=lfs -text
*.bin !text -filter -merge -diff
EOF)
)
end_test

begin_test "migrate import (identical contents, different permissions)"
(
  set -e

  # Windows lacks POSIX permissions.
  [ "$IS_WINDOWS" -eq 1 ] && exit 0

  setup_multiple_local_branches
  git checkout main

  echo "foo" >foo.dat
  git add .
  git commit -m "add file"

  chmod u+x foo.dat
  git add .
  git commit -m "make file executable"

  # Verify we have executable permissions.
  ls -la foo.dat | grep 'rwx'

  git lfs migrate import --everything --include="*.dat"

  # Verify we have executable permissions.
  ls -la foo.dat | grep 'rwx'
)
end_test

begin_test "migrate import (tags with same name as branches)"
(
  set -e

  setup_multiple_local_branches
  git checkout main

  contents="hello"
  oid=$(calc_oid "$contents")
  printf "$contents" >hello.dat
  git add .
  git commit -m "add file"

  git branch foo
  git tag foo
  git tag bar

  git lfs migrate import --everything --include="*.dat"

  [ "$(git rev-parse refs/heads/foo)" = "$(git rev-parse refs/tags/foo)" ]
  [ "$(git rev-parse refs/heads/foo)" = "$(git rev-parse refs/tags/bar)" ]

  assert_pointer "refs/heads/foo" hello.dat "$oid" 5
)
end_test

begin_test "migrate import (bare repository)"
(
  set -e

  setup_multiple_local_branches
  make_bare

  git lfs migrate import \
    --include-ref=main
)
end_test

begin_test "migrate import (nested sub-trees, no filter)"
(
  set -e

  setup_single_local_branch_deep_trees

  oid="$(calc_oid "$(git cat-file -p :foo/bar/baz/a.txt)")"
  size="$(git cat-file -p :foo/bar/baz/a.txt | wc -c | awk '{ print $1 }')"

  git lfs migrate import --everything

  assert_local_object "$oid" "$size"
)
end_test

begin_test "migrate import (prefix include(s))"
(
  set -e

  includes="foo/bar/baz/** foo/**/baz/a.txt *.txt"
  for include in $includes; do
    setup_single_local_branch_deep_trees

    oid="$(calc_oid "$(git cat-file -p :foo/bar/baz/a.txt)")"

    git lfs migrate import --include="$include"

    assert_local_object "$oid" 120

    cd ..
  done
)
end_test

begin_test "migrate import (--everything)"
(
  set -e

  setup_multiple_local_branches
  git checkout main

  main_txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  main_md_oid="$(calc_oid "$(git cat-file -p :a.md)")"
  feature_md_oid="$(calc_oid "$(git cat-file -p my-feature:a.md)")"
  main_txt_size="$(git cat-file -p :a.txt | wc -c | awk '{ print $1 }')"
  main_md_size="$(git cat-file -p :a.md | wc -c | awk '{ print $1 }')"
  feature_md_size="$(git cat-file -p my-feature:a.md | wc -c | awk '{ print $1 }')"

  git lfs migrate import --everything

  assert_pointer "main" "a.txt" "$main_txt_oid" "$main_txt_size"
  assert_pointer "main" "a.md" "$main_md_oid" "$main_md_size"
  assert_pointer "my-feature" "a.md" "$feature_md_oid" "$feature_md_size"
)
end_test

begin_test "migrate import (ambiguous reference)"
(
  set -e

  setup_multiple_local_branches

  # Create an ambiguously named reference sharing the name as the SHA-1 of
  # "HEAD".
  sha="$(git rev-parse HEAD)"
  git tag "$sha"

  git lfs migrate import --everything
)
end_test

begin_test "migrate import (--everything with args)"
(
  set -e

  setup_multiple_local_branches

  [ "$(git lfs migrate import --everything main 2>&1)" = \
    "fatal: cannot use --everything with explicit reference arguments" ]
)
end_test

begin_test "migrate import (--everything with --include-ref)"
(
  set -e

  setup_multiple_local_branches

  [ "$(git lfs migrate import --everything --include-ref=refs/heads/main 2>&1)" = \
    "fatal: cannot use --everything with --include-ref or --exclude-ref" ]
)
end_test

begin_test "migrate import (--everything with --exclude-ref)"
(
  set -e

  setup_multiple_local_branches

  [ "$(git lfs migrate import --everything --exclude-ref=refs/heads/main 2>&1)" = \
    "fatal: cannot use --everything with --include-ref or --exclude-ref" ]
)
end_test

begin_test "migrate import (--everything and --include with glob pattern)"
(
  set -e

  setup_multiple_local_branches

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  md_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.md")")"
  txt_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.txt")")"

  git lfs migrate import --verbose --everything --include='*.[mM][dD]'

  assert_pointer "refs/heads/main" "a.md" "$md_main_oid" "140"
  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "30"

  assert_local_object "$md_main_oid" "140"
  assert_local_object "$md_feature_oid" "30"
  refute_local_object "$txt_main_oid"
  refute_local_object "$txt_feature_oid"
)
end_test

begin_test "migrate import (--everything with tag pointing to tag)"
(
  set -e

  setup_multiple_local_branches

  md_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.md")")"
  txt_main_oid="$(calc_oid "$(git cat-file -p "refs/heads/main:a.txt")")"
  md_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.md")")"
  txt_feature_oid="$(calc_oid "$(git cat-file -p "refs/heads/my-feature:a.txt")")"

  git tag -a -m abc abc refs/heads/main
  git tag -a -m def def refs/tags/abc

  git lfs migrate import --verbose --everything --include='*.[mM][dD]'

  assert_pointer "refs/heads/main" "a.md" "$md_main_oid" "140"
  assert_pointer "refs/tags/abc" "a.md" "$md_main_oid" "140"
  assert_pointer "refs/tags/def" "a.md" "$md_main_oid" "140"
  assert_pointer "refs/heads/my-feature" "a.md" "$md_feature_oid" "30"

  git tag --points-at refs/tags/abc | grep -q def
  ! git tag --points-at refs/tags/def | grep -q abc

  assert_local_object "$md_main_oid" "140"
  assert_local_object "$md_feature_oid" "30"
  refute_local_object "$txt_main_oid"
  refute_local_object "$txt_feature_oid"
)
end_test

begin_test "migrate import (nested sub-trees and --include with wildcard)"
(
  set -e

  setup_single_local_branch_deep_trees

  oid="$(calc_oid "$(git cat-file -p :foo/bar/baz/a.txt)")"
  size="$(git cat-file -p :foo/bar/baz/a.txt | wc -c | awk '{ print $1 }')"

  git lfs migrate import --include="**/*ar/**"

  assert_pointer "refs/heads/main" "foo/bar/baz/a.txt" "$oid" "$size"
  assert_local_object "$oid" "$size"
)
end_test

begin_test "migrate import (handle copies of files)"
(
  set -e

  setup_single_local_branch_deep_trees

  # add the object from the sub-tree to the root directory
  cp foo/bar/baz/a.txt a.txt
  git add a.txt
  git commit -m "duplicated file"

  oid_root="$(calc_oid "$(git cat-file -p :a.txt)")"
  oid_tree="$(calc_oid "$(git cat-file -p :foo/bar/baz/a.txt)")"
  size="$(git cat-file -p :foo/bar/baz/a.txt | wc -c | awk '{ print $1 }')"

  # only import objects under "foo"
  git lfs migrate import --include="foo/**"

  assert_pointer "refs/heads/main" "foo/bar/baz/a.txt" "$oid_tree" "$size"
  assert_local_object "$oid_tree" "$size"

  # "a.txt" is not under "foo" and therefore should not be in LFS
  oid_root_after_migration="$(calc_oid "$(git cat-file -p :a.txt)")"
  [ "$oid_root" = "$oid_root_after_migration" ]
)
end_test

begin_test "migrate import (--object-map)"
(
  set -e

  setup_multiple_local_branches

  output_dir=$(mktemp -d)

  git log --all --pretty='format:%H' > "${output_dir}/old_sha.txt"
  git lfs migrate import --everything --object-map "${output_dir}/object-map.txt"
  git log --all --pretty='format:%H' > "${output_dir}/new_sha.txt"
  paste -d',' "${output_dir}/old_sha.txt" "${output_dir}/new_sha.txt" > "${output_dir}/expected-map.txt"

  diff -u <(sort "${output_dir}/expected-map.txt") <(sort "${output_dir}/object-map.txt")
)
end_test

begin_test "migrate import (--include with space)"
(
  set -e

  setup_local_branch_with_space

  oid="$(calc_oid "$(git cat-file -p :"a file.txt")")"

  git lfs migrate import --include "a file.txt"

  assert_pointer "refs/heads/main" "a file.txt" "$oid" 50
  cat .gitattributes
  if [ 1 -ne "$(grep -c "a\[\[:space:\]\]file.txt" .gitattributes)" ]; then
    echo >&2 "fatal: expected \"a[[:space:]]file.txt\" to appear in .gitattributes"
    echo >&2 "fatal: got"
    sed -e 's/^/  /g' < .gitattributes >&2
    exit 1
  fi
)
end_test

begin_test "migrate import (handle symbolic link)"
(
  set -e

  setup_local_branch_with_symlink

  txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  link_oid="$(calc_oid "$(git cat-file -p :link.txt)")"

  git lfs migrate import --include="*.txt"

  assert_pointer "refs/heads/main" "a.txt" "$txt_oid" "120"

  assert_local_object "$txt_oid" "120"
  # "link.txt" is a symbolic link so it should be not in LFS
  refute_local_object "$link_oid" "5"
)
end_test

begin_test "migrate import (commit --allow-empty)"
(
  set -e

  reponame="migrate---allow-empty"
  git init "$reponame"
  cd "$reponame"

  git commit --allow-empty -m "initial commit"

  original_head="$(git rev-parse HEAD)"
  git lfs migrate import --everything
  migrated_head="$(git rev-parse HEAD)"

  assert_ref_unmoved "HEAD" "$original_head" "$migrated_head"
)
end_test

begin_test "migrate import (multiple remotes)"
(
  set -e

  setup_multiple_remotes

  original_main="$(git rev-parse main)"

  git lfs migrate import

  migrated_main="$(git rev-parse main)"

  assert_ref_unmoved "main" "$original_main" "$migrated_main"
)
end_test

begin_test "migrate import (dirty copy, default negative answer)"
(
  set -e

  setup_local_branch_with_dirty_copy

  original_main="$(git rev-parse main)"

  echo | git lfs migrate import --everything 2>&1 | tee migrate.log
  grep "migrate: working copy must not be dirty" migrate.log

  migrated_main="$(git rev-parse main)"

  assert_ref_unmoved "main" "$original_main" "$migrated_main"
)
end_test

begin_test "migrate import (dirty copy, negative answer)"
(
  set -e

  setup_local_branch_with_dirty_copy

  original_main="$(git rev-parse main)"

  echo "n" | git lfs migrate import --everything 2>&1 | tee migrate.log
  grep "migrate: working copy must not be dirty" migrate.log

  migrated_main="$(git rev-parse main)"

  assert_ref_unmoved "main" "$original_main" "$migrated_main"
)
end_test

begin_test "migrate import (dirty copy, unknown then negative answer)"
(
  set -e

  setup_local_branch_with_dirty_copy

  original_main="$(git rev-parse main)"

  echo "x\nn" | git lfs migrate import --everything 2>&1 | tee migrate.log

  cat migrate.log

  [ "2" -eq "$(grep -o "override changes in your working copy" migrate.log \
    | wc -l | awk '{ print $1 }')" ]
  grep "migrate: working copy must not be dirty" migrate.log

  migrated_main="$(git rev-parse main)"

  assert_ref_unmoved "main" "$original_main" "$migrated_main"
)
end_test

begin_test "migrate import (dirty copy, positive answer)"
(
  set -e

  setup_local_branch_with_dirty_copy

  oid="$(calc_oid "$(git cat-file -p :a.txt)")"

  echo "y" | git lfs migrate import --everything 2>&1 | tee migrate.log
  grep "migrate: changes in your working copy will be overridden ..." \
    migrate.log

  assert_pointer "refs/heads/main" "a.txt" "$oid" "5"
  assert_local_object "$oid" "5"
)
end_test

begin_test "migrate import (non-standard refs)"
(
  set -e

  setup_multiple_local_branches_non_standard

  md_oid="$(calc_oid "$(git cat-file -p :a.md)")"
  txt_oid="$(calc_oid "$(git cat-file -p :a.txt)")"
  md_feature_oid="$(calc_oid "$(git cat-file -p my-feature:a.md)")"

  git lfs migrate import --everything

  assert_pointer "refs/heads/main" "a.md" "$md_oid" "140"
  assert_pointer "refs/heads/main" "a.txt" "$txt_oid" "120"
  assert_pointer "refs/pull/1/base" "a.md" "$md_oid" "140"
  assert_pointer "refs/pull/1/base" "a.txt" "$txt_oid" "120"

  assert_pointer "refs/heads/my-feature" "a.txt" "$txt_oid" "120"
  assert_pointer "refs/pull/1/head" "a.txt" "$txt_oid" "120"

  assert_local_object "$md_oid" "140"
  assert_local_object "$txt_oid" "120"
  assert_local_object "$md_feature_oid" "30"
)
end_test
