# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

#####################
# Description
#####################

# This file is used to send slack notifications after a job has been executed.
# It expects SLACK_INCOMING_WEBHOOK variable to be available in env
# example command: python3 utils/slack_notifier.py --job_status "${CURRENT_BUILD_RESULT}" --job_name "${JOB_NAME}" --build_url "${BUILD_URL}"

import argparse, json
import requests, os
from pathlib import Path

UTILS_DIR = Path(__file__).parent

parser = argparse.ArgumentParser()
parser.add_argument('--job_status', dest='job_status', type=str, help='current job status')
parser.add_argument('--job_name', dest='job_name', type=str, help='current job name')
parser.add_argument('--build_url', dest='build_url', type=str, help='current build url')
args = parser.parse_args()

# Set the webhook_url to the one provided by Slack when you create the webhook at https://my.slack.com/services/new/incoming-webhook/
incoming_webhook_url = os.environ.get("SLACK_INCOMING_WEBHOOK")

with open(os.path.join(UTILS_DIR,"slack-payloads/job-result.json"), 'r') as f:
    payload_data = json.load(f)

    for block in payload_data["blocks"]:

        if "JOB_NAME" in str(block):
            replace_text = block["text"]["text"]
            replace_text=replace_text.replace("JOB_NAME",args.job_name)
            block["text"]["text"]=replace_text

        elif "JOB_STATUS" in str(block):
            status_text = ""
            if (args.job_status).lower() == "success":
                status_text = "correct"
            else:
                status_text = "error-cross"

            replace_text = block["text"]["text"]
            replace_text=replace_text.replace("JOB_STATUS",args.job_status)
            replace_text=replace_text.replace("STATUS_ICON",status_text)
            block["text"]["text"]=replace_text

        elif "JOB_URL" in str(block):
            replace_text = block["text"]["text"]
            replace_text=replace_text.replace("JOB_URL",args.build_url)
            block["text"]["text"]=replace_text

print(json.dumps(payload_data,indent=2))
response = requests.post(
    incoming_webhook_url, data=json.dumps(payload_data),
    headers={'Content-Type': 'application/json'}
)
if response.status_code != 200:
    raise ValueError(
        'Request to slack returned an error %s, the response is:\n%s'
        % (response.status_code, response.text)
    )
else:
    print("payload sent!")
