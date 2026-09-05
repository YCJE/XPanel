#!/bin/sh

# Start fail2ban
[ $XPANEL_ENABLE_FAIL2BAN == "true" ] && fail2ban-client -x start

# Run xpanel
exec /app/xpanel
