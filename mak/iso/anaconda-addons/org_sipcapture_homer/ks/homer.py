#
# Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Author Konstantin S. Vishnivetsky kvishnivetsky@sipcapture.org
# Licensed to the User under the GPL license.
#

from pyanaconda.addons import AddonData
from pykickstart.options import KSOptionParser

__all__ = ["HomerData"]

class HomerData(AddonData):

	def __init__(self, name):

		AddonData.__init__(self, name)
		self.homer_user = ""
		self.homer_password = ""
		self.homer_database_name = ""
		self.homer_database_host = ""
		self.homer_webserver_type = ""

	def handle_header(self, lineno, args):

		op = KSOptionParser()
		op.add_option("--homer-user", action="store_true", default=False,
		dest="homer_user", help="Homer database user name")
		op.add_option("--homer-password", action="store_true", default=False,
		dest="homer_password", help="Homer database user password")
		op.add_option("--homer-database-name", action="store_true", default=False,
		dest="homer_database_name", help="Homer database name")
		op.add_option("--homer-database-host", action="store_true", default=False,
		dest="homer_database_host", help="Homer database host")
		op.add_option("--homer-timezone", action="store_true", default=False,
		dest="homer_timezone", help="Homer database timezone")
		op.add_option("--web-server-type", action="store_true", default=False,
		dest="homer_webserver_type", help="Homer web-server type: httpd or nginx")


		(opts, extra) = op.parse_args(args=args, lineno=lineno)
#
#		# Reject any additoinal arguments. Since AddonData.handle_header
#		# rejects any arguments, we can use it to create an error message
#		# and raise an exception.
		if extra:
			AddonData.handle_header(self, lineno, extra)
		# Store the result of the option parsing
		self.homer_user = opts.homer_user
		self.homer_password = opts.homer_password
		self.homer_database_name = opts.homer_database_name
		self.homer_database_host = opts.homer_database_host
		self.homer_timezone = opts.homer_timezone
		self.homer_webserver_type = opts.homer_webserver_type

	def handle_line(self, line):

#		if self.text is "":
#		self.text = line.strip()
#		else:
#		self.text += " " + line.strip()
		pass

	def setup(self, storage, ksdata, instclass):

		pass

	def execute(self, storage, ksdata, instclass, users):

		pass

	def __str__(self):

		addon_str = "%%addon %s" % self.name
		addon_str = " --homer-user %s" % self.homer_user
		addon_str = " --homer-password %s" % self.homer_password
		addon_str = " --homer-database-name %s" % self.homer_database_name
		addon_str = " --homer-database-host %s" % self.homer_database_host
		addon_str = " --homer-timezone %s" % self.homer_timezone
		addon_str = " --web-server-type %s" % self.homer_webserver_type
		addon_str += "\nend\n"
		return addon_str
