This little proxy will resolve TLS and then forward to a localhost port based on the domain name root.

deploy.sh assumes you have an ubuntu EC2 instance, and that you have started the reverse_proxy.service on the machine.

It also assumes SSL Certs were created via Certbot
