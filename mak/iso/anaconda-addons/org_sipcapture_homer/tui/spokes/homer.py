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
	"""Auxiliary class for storing data"""

	def __init__(self):
		"""Trivial constructor just defining the fields that will store data"""
		self.homer_user = ""
		self.homer_password = ""
		self.homer_database_name = "homer"
		self.homer_database_host = "localhost"
		self.homer_webserver_type = ""

class HomerSettingsSpoke(FirstbootSpokeMixIn, EditTUISpoke):

	title = N_("Homer 5")
	category = "localization"

	# simple RE used to specify we only accept a single word as a valid input
	_valid_input = re.compile(r'\w')

	edit_fields = [
		Entry("DB User Name", "homer_user", _valid_input, True),
		Entry("DB User Password", "homer_password", EditTUISpoke.PASSWORD, True),
		Entry("DB Name", "homer_database_name", _valid_input, True),
		Entry("DB Host", "homer_database_host", _valid_input, True),
		Entry("DB WEB Server Type", "homer_webserver_type", _valid_input, True),
		]

	def __init__(self, app, data, storage, payload, instclass):
		EditTUISpoke.__init__(self, app, data, storage, payload, instclass)
		self.args = _EditData()

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

	def prompt(self, args=None):
		return "1) DB User Name%s\n2) DB User Password%s\n3) DB name%s\n4) DB host%s\n5) WEB Server Type%s\n%s: " % (
			((" [%s]" % self.args.homer_user) if self.args.homer_user else "[!]"),
			((" [*****]") if self.args.homer_password else " [!]"),
			((" [%s]" % self.args.homer_database_name) if self.args.homer_database_name else "[!]"),
			((" [%s]" % self.args.homer_database_host) if self.args.homer_database_host else "[!]"),
			((" [%s]" % self.args.homer_webserver_type) if self.args.homer_webserver_type else "[!]"),
			"Please, make your choice from above ['q' to quit | 'c' to return to main menu]: "
			)

	def execute(self):
		pass

	@property
	def completed(self):
		return bool(self.data.addons.org_sipcapture_homer.homer_user and self.data.addons.org_sipcapture_homer.homer_password
			and self.data.addons.org_sipcapture_homer.homer_database_name and self.data.addons.org_sipcapture_homer.homer_database_host
			and self.data.addons.org_sipcapture_homer.homer_webserver_type);

	@property
	def status(self):
		return "%sonfigured" % ("C" if self.data.addons.org_sipcapture_homer.homer_user and self.data.addons.org_sipcapture_homer.homer_password
			and self.data.addons.org_sipcapture_homer.homer_database_name and self.data.addons.org_sipcapture_homer.homer_database_host
			and self.data.addons.org_sipcapture_homer.homer_webserver_type else "Not c");
