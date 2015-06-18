[![Logo](http://sipcapture.org/data/images/sipcapture_header.png)](http://sipcapture.org)

## HOMER 5
#### 100% Open-Source Modular SIP & VoIP Capture Server & Agent

HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling protocol.

Powered at the core by [SIPCAPTURE](http://kamailio.org/docs/modules/stable/modules/sipcapture.html) Module for industry-standard [Kamailio](http://kamailio.org), HOMER provides virtually unlimited scope for granular capture [configuration](https://github.com/sipcapture/homer/blob/master/configs/kamailio.cfg) either stand-alone or using our companion [Capture Agent](https://github.com/sipcapture/captagent) Project.

HOMER 5 User-Interface is developed using standard Angular JS, easily extensible and with all functionality moved to specialized and customizable widgets feeding and displaying correlated data from internal and external data sources such as InfluxDB and Elasticsearch. HOMER allows users to define and generate statistic from its capture dialplan including fully programmable threshold triggering and alarming, providing plenty of space for tailored configurations and logic customizations.

HOMER is already used by large voice networks, voip service providers and traffic carriers worldwide, has been implemented as a service in 3rd party voice platforms and is suitable for production. Contact the team for your basic and advanced needs or leverage the experience of our great community by joining our [mailing-list](http://groups.google.com/group/homer-discuss). 

## Components
The HOMER Application is composed by:

* [Homer-API](https://github.com/sipcapture/homer-api)
  * Sipcapture Backend connector and API Component
* [Homer-UI](https://github.com/sipcapture/homer-ui)
  * Sipcapture Frontend & JS User-Interface Component
* Kamailio-Dev
  * Sipcapture HEP Capture Server _(sipcapture module)_


## Installation

Please follow our [Setup Guide](https://github.com/sipcapture/homer/blob/homer5/INSTALL.md) to get started.


### Need Support?
For support, installations, customizations or commercial requests please contact: support@sipcapture.org

For community updates, user discussion and experience exchange please join our users   [Mailing-List](https://groups.google.com/forum/#!forum/homer-discuss)

[![HomerFlow](http://i.imgur.com/U7UBI.png)](http://sipcapture.org)

##### HOMER's [Captagent](http://github.com/sipcapture/captagent) is now also available on [Github](http://github.com/sipcapture/captagent)

### Developers
Contributors and Contributions to our project are always welcome! If you intend to participate and help us improve HOMER by sending patches, we kindly ask you to sign a standard [CLA (Contributor License Agreement)](http://cla.qxip.net) which enables us to distribute your code alongside the project without restrictions present or future. It doesn’t require you to assign to us any copyright you have, the ownership of which remains in full with you. Developers can coordinate with the existing team via the [homer-dev](http://groups.google.com/group/homer-dev) mailing list. If you'd like to join our internal team and volounteer to help with the project's many needs, feel free to contact us anytime!




### License & Copyright

*Homer components are released under GNU AGPLv3 license*

*Captagent is released under GNU GPLv3 license*

*(C) 2008-2015 SIPCAPTURE & QXIP BV*

----------

##### If you use HOMER in production, please consider supporting the project with a [Donation](https://www.paypal.com/cgi-bin/webscr?cmd=_donations&business=donation%40sipcapture%2eorg&lc=US&item_name=SIPCAPTURE&no_note=0&currency_code=EUR&bn=PP%2dDonationsBF%3abtn_donateCC_LG%2egif%3aNonHostedGuest)

[![Donate](https://www.paypalobjects.com/en_US/i/btn/btn_donateCC_LG.gif)](https://www.paypal.com/cgi-bin/webscr?cmd=_donations&business=donation%40sipcapture%2eorg&lc=US&item_name=SIPCAPTURE&no_note=0&currency_code=EUR&bn=PP%2dDonationsBF%3abtn_donateCC_LG%2egif%3aNonHostedGuest)


