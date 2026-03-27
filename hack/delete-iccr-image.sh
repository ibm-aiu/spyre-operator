#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

usage() {
	echo "Usage: $0 [spyre-operator/<repo>] [<Days>]"
	echo "	Delete images in the docker.io/spyre-operator/<repo> that are older than <Days>,"
	echo " 	exclude tags with the pattern: 0.1.0 0.1.0-dev 0.1.0-dev-amd64 2.4.0-rc.1 latest-pr"
	echo " 	Optional --dry-run Default: true Will not delete but only show images will be deleted"
}

if [ $? -ne 0 ]; then
	echo "Failed to parse options" >&2
	echo
	usage
	exit 1
fi

DRY_RUN=${DRY_RUN:-false}
EPOC_NOW=$(date -u +%s)
KEEP=()   # list of repo@digests to keep
DELETE=() # list of repo@digests to delete
BATCH=${BATCH_DELETE_SIZE:-10}
declare -a POSITIONAL_ARGS
declare -A SEEN # list of checked digests:value

while [[ $# -gt 0 ]]; do
	case $1 in
	--dry-run)
		DRY_RUN=true
		shift
		;;
	--)
		shift
		POSITIONAL_ARGS+=("$@")
		break
		;;
	-*)
		echo "Unknown option: $1"
		exit 1
		;;
	*)
		POSITIONAL_ARGS+=("$1")
		shift
		;;
	esac
done

# print usage if parameters not specified
[[ ${#POSITIONAL_ARGS[@]} -ne 2 ]] && usage && exit

REPO=${POSITIONAL_ARGS[0]}     # namespace/repository
DAYS_OLD=${POSITIONAL_ARGS[1]} # age in days
CUTOFF_EPOC=$(date -u --date="${DAYS_OLD} days ago" +%s)

# keep images with tags that have release, *-dev, etc patterns
is_excluded_tag() {
	# tags is array
	local tags=$1
	for t in "${tags[@]}"; do
		[[ ${t} =~ ^(latest-pr|fast|stable|[0-9]+\.[0-9]+\.[0-9])+(-(amd64|ppc64le|arm64|s390x|dev|rc\.[0-9]+)(-(amd64|ppc64le|arm64|s390x))?)?$ ]] && return 0 || return 1
	done
}

days_old() {
	local created_epoc=$1
	if [[ ${created_epoc} == "0" ]]; then
		echo "0"
	else
		diff_sec=$((EPOC_NOW - created_epoc))
		echo $((diff_sec / 86400))
	fi
}

has_access_or_exit() {
	# test namespace access
	[ -z "$(ibmcloud cr namespace-list | grep -w "spyre-operator")" ] && echo 'Please check iccr namespce access ("ibmcloud login" and "ibmcloud cr login" may be required)' && exit
}

# Get the list of manifest lists informat of repo:tag|manifest_type|digest
# for example:
# docker.io/spyre-operator/aiu_operator/aiu-operator-catalog:2.3.0-dev sha256:8fdd17bacd8cd3d3964ff01f546fa77b19f723b4ffac73cdcbceaefd86567d28 OCI Image Index v1
list_manifests() {
	local repo=$1
	ibmcloud cr image-digests -q --restrict "${repo}" --format '{{.Repository}}|{{.Tags}}|{{.Digest}}|{{.ManifestType}}|{{.Created}}' | grep -E 'OCI Image Index|Docker Manifest List'
}

list_images() {
	local repo=$1
	ibmcloud cr image-digests -q --restrict "${repo}" --format '{{.Repository}}|{{.Tags}}|{{.Digest}}|{{.ManifestType}}|{{.Created}}' | grep -E 'OCI Image Manifest|Docker Image Manifest V2'
}

total_digests() {
	local repo=$1
	ibmcloud cr image-digests -q --restrict "${repo}" --format '{{.Repository}}|{{.Tags}}|{{.Digest}}|{{.ManifestType}}|{{.Created}}' | wc -l
}

get_digests_from_manifest_list() {
	# example of image_digest:
	# docker.io/spyre-operator/aiu_operator/aiu-operator@sha256:8fdd17bacd8cd3d3964ff01f546fa77b19f723b4ffac73cdcbceaefd86567d28
	local image_digest=$1 list=()
	for d in $(ibmcloud cr manifest-inspect "${image_digest}" -q | jq -r '.manifests[].digest'); do
		list+=("${image_digest%@*}@${d}")
	done
	echo "${list[@]}"
}
is_image_manifest_type() {
	# Manifest types:
	#  Docker Image Manifest V2, Schema 2
	#  Docker Manifest List
	#  OCI Image Index v1
	#  OCI Image Manifest v1
	local mt=$1
	[[ ${mt} =~ "Docker Image Manifest" ]] || [[ ${mt} =~ "OCI Image Manifest" ]] && return 0 || return 1
}
check_digest() {
	local line repo_name tags digest children_digests created_epoc date type
	line=$1
	# line example docker.io/spyre-operator/spyre-operator-catalog|[0.1.0-dev-f720bca]|sha256:e9a1c45cb6ac3159d4c53cc59dd017a1a0c5679c6c5161fa6a448323caee4244|Docker Manifest List|0
	repo_name=$(echo "${line}" | cut -d"|" -f1)           # docker.io/spyre-operator/aiu_operator/aiu-operator
	tags=($(echo "${line}" | cut -d"|" -f2 | tr -d "[]")) # 2.5.0-dev-s390x or multiple tags
	digest=$(echo "${line}" | cut -d"|" -f3)              # sha256:2f92a5e4b1f29f6e8ac3d57c11b6e1a62b1ac27260c91b679e7c3d1f7d3bd445
	type="$(echo "${line}" | cut -d"|" -f4)"              # Docker Image Manifest V2, Schema 2
	created_epoc="$(echo "${line}" | cut -d"|" -f5)"      # 1749543620 or (0 if not an image manifest type)

	# manifest is a image type
	if is_image_manifest_type "${type}"; then
		children_digests=() # image manifest has no children
	else                 # manifest is a list type
		# Get the children image and date their Created date
		children_digests=($(get_digests_from_manifest_list "${repo_name}@${digest}"))
		# date is in ISO8601 format 2025-05-08T02:11:48.11378324Z not epoch time
		date=$(ibmcloud cr image-inspect "${children_digests[0]}" --format '{{.Created}}')
		created_epoc=$(date -d "${date}" +%s)
	fi

	# tag is one of the release or dev pattern keep the and its children images
	if is_excluded_tag "${tags[@]}"; then
		KEEP+=("${repo_name}@${digest}")
		KEEP+=("${children_digests[@]}")
		echo "Keep ${repo_name}:${tags[*]} ${digest} [${type}]"
		[[ ${#children_digests[@]} -gt 0 ]] && printf "  %s\n" "${children_digests[@]}"
		echo
		# delete the ml and children images which are older then DAYS
	elif [[ ${created_epoc} -lt ${CUTOFF_EPOC} ]]; then
		DELETE+=("${repo_name}@${digest}") # ml
		DELETE+=("${children_digests[@]}") # children
		echo "Delete ${repo_name}:${tags[*]} ${digest} [${type}] created $(days_old "${created_epoc}") days ago"
		[[ ${#children_digests[@]} -gt 0 ]] && printf "  %s\n" "${children_digests[@]}"
		echo
	else
		# tag is NOT one of the release or dev pattern and children are under DAYS old
		KEEP+=("${repo_name}@${digest}")
		KEEP+=("${children_digests[@]}")
		echo "Keep ${repo_name}:${tags[*]} ${digest} [${type}] $(days_old "${created_epoc}") days ago"
		[[ ${#children_digests[@]} -gt 0 ]] && printf "  %s\n" "${children_digests[@]}"
		echo
	fi

	SEEN["${repo_name}@${digest}"]=1
	for c in "${children_digests[@]}"; do
		SEEN["${c}"]=1
	done

}
# look for old manifest lists(ml) and their children images or images in form of repo@digest to keep or delete
check_manifest_list() {
	local repo=$1 # spyre-operator/aiu_operator/aiu-operator
	while read -r line; do
		check_digest "${line}"
	done < <(list_manifests "${repo}")
}

check_remaining_image_digests() {
	local repo=$1 repo_name digest
	while read -r line; do
		# line example docker.io/spyre-operator/spyre-operator-catalog|[0.1.0-dev-c09c64a-s390x]|sha256:f7461227461db503b0ba3b7776d016c6c4e844fe6352d84e46c1743b30f3f448|Docker Image Manifest V2, Schema 2|1752230455
		repo_name=$(echo "${line}" | cut -d"|" -f1) # docker.io/spyre-operator/spyre-operator-catalog
		digest=$(echo "${line}" | cut -d"|" -f3)    # sha256:f7461227461db503b0ba3b7776d016c6c4e844fe6352d84e46c1743b30f3f448
		# check only the image digests which were not seen in the previously
		if [[ -z ${SEEN["${repo_name}@${digest}"]} ]]; then
			check_digest "${line}"
		fi
	done < <(list_images "${repo}")
}

delete_images_in_batche() {
	local batch batch_size images len
	# Check if parameters are provided
	if [ $# -lt 2 ]; then
		echo "Usage: $0 batch_size element1 element2 ..."
		return 1
	fi
	batch_size=$1
	shift
	images=("$@")
	len=${#images[@]}

	# Loop through the array in batches
	echo "Batch delete size: ${batch_size}"
	for ((i = 0; i < len; i += batch_size)); do
		echo "Batch $((i / batch_size + 1)):"
		batch=()
		for ((j = i; j < i + batch_size && j < len; j++)); do
			batch+=("${images[j]}")
		done
		if [ "${DRY_RUN}" = false ]; then
			ibmcloud cr image-rm "${batch[@]}"
		else
			echo "ibmcloud cr image-rm" "${batch[*]}"
		fi
		echo "----------------"
	done
}

# Sometimes digests could have multiple tags which could be both keep and delete.
# For example: A digest may be in manifest lists with a tag(eg. 0.1.0) that should be kept and at the same,
# also belong to manifest lists that have tags (0.2.0-dev-5ad8831, 0.1.1-dev-219ca76) which maybe subject to delete.
# In this case, we should keep the digest because of the 0.1.0 tag otherwise the manifest list will be missing an image digest.
remove_keep_from_delete() {
	local remove
	local temp
	echo "Checking again for any digest that should not be deleted.."
	for d in "${DELETE[@]}"; do
		remove=false
		for k in "${KEEP[@]}"; do
			if [[ ${d} == "${k}" ]]; then
				remove=true
				echo "${d}"
				break
			fi
		done
		if [[ ${remove} == false ]]; then
			temp+=("${d}")
		fi
	done
	DELETE=("${temp[@]}")
	echo "finished checking"
	echo
}

# Main starts
has_access_or_exit
echo "DRY_RUN is set ${DRY_RUN}"

# STEP 1: Compile a list of manifest list and images(repo@digests) to keep and delete
echo "Find manifests lists in ${REPO} older than ${DAYS_OLD} days to delete"
check_manifest_list "${REPO}"
echo "Total digests seen: ${#SEEN[@]}"
echo "Total digests to keep: ${#KEEP[@]}"
echo "Total digests to delete: ${#DELETE[@]}"
echo

# STEP 2: Compile a list of images to keep and delete.
echo "Find images ${REPO} older than ${DAYS_OLD} days to delete"
check_remaining_image_digests "${REPO}"
echo "Total digests seen: ${#SEEN[@]}"
echo "Total digests to keep: ${#KEEP[@]}"
echo "Total digests to delete: ${#DELETE[@]}"
echo "Total digest: $(total_digests "${REPO}")"
echo

remove_keep_from_delete

# STEP 3: Delete
if [[ ${#DELETE[@]} -eq 0 ]]; then
	echo "Nothing to delete."
else
	delete_images_in_batche "${BATCH}" "${DELETE[@]}"
fi
