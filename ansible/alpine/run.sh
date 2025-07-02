cd /opt/FacilitatorBot/ansible
ansible-playbook -i inventory.yml deploy_bot.yaml --connection=local

rc-service facilitator-bot restart

# Find the PID of the process
#pid=$(ps -ef | grep facilitator-bot | awk '{print $1}')

# Terminate the process gracefully
#kill $pid

#/opt/FacilitatorBot/bin/facilitator-bot &