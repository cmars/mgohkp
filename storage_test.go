/*
   Hockeypuck - OpenPGP key server
   Copyright (C) 2012-2014  Casey Marshall

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, version 3.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package mgohkp

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	stdtesting "testing"

	"github.com/facebookgo/mgotest"
	"github.com/hockeypuck/testing"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/hockeypuck/hkp.v0"
	"gopkg.in/hockeypuck/openpgp.v0"
)

func Test(t *stdtesting.T) { gc.TestingT(t) }

type MgoSuite struct {
	storage *storage
	mgoSrv  *mgotest.Server
	session *mgo.Session
	srv     *httptest.Server
}

var _ = gc.Suite(&MgoSuite{})

func (s *MgoSuite) SetUpTest(c *gc.C) {
	s.mgoSrv = mgotest.NewStartedServer(c)
	s.session = s.mgoSrv.Session()
	st, err := NewStorage(s.session)
	c.Assert(err, gc.IsNil)
	s.storage = st.(*storage)

	r := httprouter.New()
	handler := hkp.NewHandler(s.storage)
	handler.Register(r)
	s.srv = httptest.NewServer(r)
}

func (s *MgoSuite) TearDownTest(c *gc.C) {
	s.srv.Close()
	s.session.Close()
	s.mgoSrv.Stop()
}

func (s *MgoSuite) addKey(c *gc.C, keyname string) {
	keytext, err := ioutil.ReadAll(testing.MustInput(keyname))
	c.Assert(err, gc.IsNil)
	res, err := http.PostForm(s.srv.URL+"/pks/add", url.Values{
		"keytext": []string{string(keytext)},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(res.StatusCode, gc.Equals, http.StatusOK)
	defer res.Body.Close()
	_, err = ioutil.ReadAll(res.Body)
	c.Assert(err, gc.IsNil)
}

func (s *MgoSuite) TestMD5(c *gc.C) {
	res, err := http.Get(s.srv.URL + "/pks/lookup?op=hget&search=da84f40d830a7be2a3c0b7f2e146bfaa")
	c.Assert(err, gc.IsNil)
	res.Body.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(res.StatusCode, gc.Equals, http.StatusNotFound)

	s.addKey(c, "sksdigest.asc")
	session, coll := s.storage.c()
	defer session.Close()
	var doc keyDoc
	err = coll.Find(bson.D{{"md5", "da84f40d830a7be2a3c0b7f2e146bfaa"}}).One(&doc)
	c.Assert(err, gc.IsNil)

	res, err = http.Get(s.srv.URL + "/pks/lookup?op=hget&search=da84f40d830a7be2a3c0b7f2e146bfaa")
	c.Assert(err, gc.IsNil)
	armor, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(res.StatusCode, gc.Equals, http.StatusOK)

	keys := openpgp.MustReadArmorKeys(bytes.NewBuffer(armor)).MustParse()
	c.Assert(keys, gc.HasLen, 1)
	c.Assert(keys[0].ShortID(), gc.Equals, "ce353cf4")
	c.Assert(keys[0].UserIDs, gc.HasLen, 1)
	c.Assert(keys[0].UserIDs[0].Keywords, gc.Equals, "Jenny Ondioline <jennyo@transient.net>")
}

func (s *MgoSuite) TestAddDuplicates(c *gc.C) {
	res, err := http.Get(s.srv.URL + "/pks/lookup?op=hget&search=da84f40d830a7be2a3c0b7f2e146bfaa")
	c.Assert(err, gc.IsNil)
	res.Body.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(res.StatusCode, gc.Equals, http.StatusNotFound)

	for i := 0; i < 10; i++ {
		s.addKey(c, "sksdigest.asc")
	}

	session, coll := s.storage.c()
	defer session.Close()
	n, err := coll.Find(bson.D{{"md5", "da84f40d830a7be2a3c0b7f2e146bfaa"}}).Count()
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 1)
}