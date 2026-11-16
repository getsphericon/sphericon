package db_test

import (
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contacttag"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/tagname"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The rows of testdata/tag_name_key.
const (
	acmeID, globexID                = 1, 2
	annID, bobID, cleoID            = 1, 2, 3
	vipID, leadID, cyrillicTagID    = 1, 4, 5
	globexVIPID                     = 7
	vipName, leadName, cyrillicName = "VIP", "\u00a0Lead\t", "ТЕГ"
)

// storedTag is one Tag as the migrated schema holds it.
type storedTag struct {
	id, workspaceID int64
	name, nameKey   string
}

// held is one Tag a Contact holds.
type held struct{ contactID, tagID int64 }

// A Tag is matched by its name's key (#1342): the migration fills the key of every existing Tag as
// tagname.Key computes it, also where lower() would not (the name column collates as "C"), merges
// the Tags of a Workspace whose names share a key into the oldest, which keeps its name and takes
// over the others' Contacts, and leaves another Workspace's Tag of the same name alone.
func TestTheTagNameKeyMigrationKeysExistingTagsAndMergesTheirCaseDuplicates(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "tag_name_key", filepath.Join("testdata", "tag_name_key"))
	testhelper.CollateC(t, sqlDB, "tags", "name")

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	client := entclient.Core.New(sqlDB)
	tags, err := client.Tag.Query().Order(ent.Asc(tag.FieldID)).All(t.Context())
	require.NoError(t, err)
	links, err := client.ContactTag.Query().Order(ent.Asc(contacttag.FieldContactID), ent.Asc(contacttag.FieldTagID)).All(t.Context())
	require.NoError(t, err)

	gotTags := lo.Map(tags, func(s *ent.Tag, _ int) storedTag {
		return storedTag{id: s.ID, workspaceID: s.WorkspaceID, name: s.Name, nameKey: s.NameKey}
	})
	gotLinks := lo.Map(links, func(l *ent.ContactTag, _ int) held { return held{l.ContactID, l.TagID} })
	vipKey, leadKey, cyrillicKey := tagname.Key(vipName), tagname.Key(leadName), tagname.Key(cyrillicName)
	assert.Equal(t, []storedTag{
		{id: vipID, workspaceID: acmeID, name: vipName, nameKey: vipKey},
		{id: leadID, workspaceID: acmeID, name: leadName, nameKey: leadKey},
		{id: cyrillicTagID, workspaceID: acmeID, name: cyrillicName, nameKey: cyrillicKey},
		{id: globexVIPID, workspaceID: globexID, name: vipName, nameKey: vipKey},
	}, gotTags)
	assert.Equal(t, []held{
		{annID, vipID}, {annID, leadID}, {annID, cyrillicTagID},
		{bobID, vipID},
		{cleoID, globexVIPID},
	}, gotLinks)
}
