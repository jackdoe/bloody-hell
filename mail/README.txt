

cat << EOL | go run *.go
{
   "Accounts" : {
      "List": [
          {
             "User" : "jack@sofialondonmoskva.com",
             "StrInboxes" : [
                "INBOX"
             ],
             "Password" : "xxxyyyzzz",
             "Server" : "imap.google.com:993",
             "Label" : "gmail"
          }
      ]
   }
}
EOL

# tail -f ~/.bloody-hell/log.txt
