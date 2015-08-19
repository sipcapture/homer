#
# Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Author Konstantin S. Vishnivetsky kvishnivetsky@sipcapture.org
# Licensed to the User under the GPL license.
#

_ = lambda x: x
N_ = lambda x: x

import re

from pyanaconda.ui.tui.spokes import EditTUISpoke
from pyanaconda.ui.tui.spokes import EditTUISpokeEntry as Entry
from pyanaconda.ui.common import FirstbootSpokeMixIn

__all__ = ["HomerSettingsSpoke"]

class _EditData(object):
	"""Auxiliary class for storing data from the example EditSpoke"""

	def __init__(self):
		"""Trivial constructor just defining the fields that will store data"""
		self.homer_user = ""
		self.homer_password = ""
		self.homer_database_name = ""
		self.homer_database_host = ""
		self.homer_webserver_type = ""

class HomerSettingsSpoke(FirstbootSpokeMixIn, EditTUISpoke):

	title = N_("Homer 5")
	category = "localization"

	# simple RE used to specify we only accept a single word as a valid input
	_valid_input = re.compile(r'\w+')

	edit_fields = [
		Entry("DB User Name", "homer_user", _valid_input, True),
		Entry("DB User Password", "homer_password", EditTUISpoke.PASSWORD, True),
		Entry("DB Name", "self.homer_database_name", _valid_input, True),
		Entry("DB Host", "self.homer_database_host", _valid_input, True),
		Entry("DB WEB Server Type", "self.homer_webserver_type", _valid_input, True),
		]

	def __init__(self, app, data, storage, payload, instclass):
		EditTUISpoke.__init__(self, app, data, storage, payload, instclass)
		self.args = _EditData()
		self.homer_user = "homer_user"
		self.homer_password = "homer_password"
		self.homer_database_name = "homer"
		self.homer_database_host = "localhost"
		self.homer_webserver_type = "httpd"

	def initialize(self):
		EditTUISpoke.initialize(self)

	def refresh(self, args=None):
		self.homer_user = self.data.addons.org_sipcapture_homer.homer_user
		self.homer_password = self.data.addons.org_sipcapture_homer.homer_password
		self.homer_database_name = self.data.addons.org_sipcapture_homer.homer_database_name
		self.homer_database_host = self.data.addons.org_sipcapture_homer.homer_database_host
		self.homer_webserver_type = self.data.addons.org_sipcapture_homer.homer_webserver_type
		return True

	def apply(self):
		self.data.addons.org_sipcapture_homer.homer_user = self.homer_user
		self.data.addons.org_sipcapture_homer.homer_password = self.homer_password
		self.data.addons.org_sipcapture_homer.homer_database_name = self.homer_database_name
		self.data.addons.org_sipcapture_homer.homer_database_host = self.homer_database_host
		self.data.addons.org_sipcapture_homer.homer_webserver_type = self.homer_webserver_type

	def execute(self):
		pass

	@property
	def completed(self):
		return False;

	@property
	def status(self):
		return "Not configured";
