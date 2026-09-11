from pathlib import Path
import subprocess, tarfile, io, hashlib, json, difflib
import argparse
parser = argparse.ArgumentParser(description='Replay Seedance migration separately from the recorded existing Link host baseline.')
parser.add_argument('destination', type=Path)
args = parser.parse_args()
root = Path(__file__).resolve().parents[1]
dest = args.destination.resolve()
if dest.exists(): raise SystemExit('Destination already exists; refusing to overwrite it')
dest.mkdir()
def git(*args):return subprocess.check_output(['git','-C',str(root),*args])
upstream='3a9f41ee85cc369f5b8d7fe6e62ff4e7bf3a9ec8';baseline='e5b6c83a9513ff2a9585cde4009d6a515b829e49'
scopes=['common','constant','controller','dto','i18n','logger','middleware','model','oauth','pkg','plugins','relay','relaykit','router','service','setting','types','go.mod','go.sum']
available=set(git('ls-tree','--name-only',upstream).decode().splitlines())
archive=git('archive',upstream,*[p for p in scopes if p in available])
with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
 for member in tar.getmembers():
  if member.issym() or member.islnk() or '..' in Path(member.name).parts:raise RuntimeError('Unsafe archive member')
 tar.extractall(dest)
# Existing Link host prerequisites are recorded separately from the new
# extension patch. This does not assert that these old local changes are new
# plugin code, nor that the extension runs without the existing Link host.
support=git('diff','--binary',upstream,baseline,'--',*scopes)
(dest/'host-baseline.patch').write_bytes(support)
subprocess.run(['git','apply','--check',str(dest/'host-baseline.patch')],cwd=dest,check=True)
subprocess.run(['git','apply',str(dest/'host-baseline.patch')],cwd=dest,check=True)
exact={
'constant/channel.go','constant/seedance_channel.go','model/channel_asset_credential_storage.go','model/channel_model_mapping.go','model/model_mapping.go','model/public_image_model_api.go','model/channel_moxing_0818_test.go',
'controller/channel_test_internal_test.go','controller/funcloud_create_integration_test.go','controller/funcloud_hosted_create_integration_test.go',
'service/asset_channel_connectivity_unsupported_test.go','service/seedance_plugin_fixture_test.go',

'controller/channel.go','controller/channel_authz.go','controller/channel_asset_tenant_boundary_test.go',
'controller/model_seedance_catalog_test.go','controller/model_retired_seedance_catalog_test.go','controller/model_list_test.go','controller/task_public_error_test.go',
'model/channel.go','model/task_plugin.go','model/channel_asset_credential.go','model/channel_asset_tenant_boundary_test.go',
'model/pricing_seedance_billing_test.go','model/pricing_endpoint_test.go',
'middleware/seedance_contract_channel_test.go','router/channel-router.go',
'pkg/publicmodel/video.go','pkg/publicmodel/media_test.go',
'service/asset_service.go','service/asset_group_policy.go','service/asset_group_policy_test.go','service/channel_default_asset_group.go',
'relaykit/dto/moxing_video_models.go',
}
prefixes=('controller/seedance','model/seedance','model/channel_seedance','pkg/jsplugin/seedance','pkg/seedanceplugin/','pkg/publicmodel/','plugins/seedance','relay/channel/task/seedance/')
tracked=set(git('ls-files').decode().splitlines())
untracked=set(git('ls-files','--others','--exclude-standard').decode().splitlines())
selected=sorted(p for p in tracked|untracked if p in exact or p.startswith(prefixes))
patch=[];changes=[]
for name in selected:
 old=dest/name;new=root/name
 before=old.read_bytes() if old.is_file() else b'';after=new.read_bytes() if new.is_file() else b''
 if before==after:continue
 beforeText=before.decode();afterText=after.decode()
 patch.extend(difflib.unified_diff(beforeText.splitlines(True),afterText.splitlines(True),fromfile='a/'+name if old.exists() else '/dev/null',tofile='b/'+name if new.exists() else '/dev/null'))
 changes.append({'path':name,'before':hashlib.sha256(before).hexdigest(),'after':hashlib.sha256(after).hexdigest() if new.exists() else None})
patch=''.join(patch).encode();(dest/'extension.patch').write_bytes(patch)
subprocess.run(['git','apply','--check',str(dest/'extension.patch')],cwd=dest,check=True)
subprocess.run(['git','apply',str(dest/'extension.patch')],cwd=dest,check=True)
manifest={'upstream':upstream,'local_host_baseline':baseline,'common_ancestor':git('merge-base',upstream,baseline).decode().strip(),
 'host_baseline_patch_sha256':hashlib.sha256(support).hexdigest(),
 'host_baseline_files':git('diff','--name-status',upstream,baseline,'--',*scopes).decode().splitlines(),
 'extension_patch_sha256':hashlib.sha256(patch).hexdigest(),'extension_files':changes}
(dest/'replay-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
print(json.dumps({'destination':str(dest),'baseline_paths':len(manifest['host_baseline_files']),'extension_paths':len(changes),'patch_sha256':manifest['extension_patch_sha256']}))
