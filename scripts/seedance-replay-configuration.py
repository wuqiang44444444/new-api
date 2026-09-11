from pathlib import Path
import subprocess,tarfile,io,hashlib,json,difflib
import argparse
parser=argparse.ArgumentParser(description='Replay the bounded Seedance configuration extension on the recorded upstream baseline.')
parser.add_argument('destination', type=Path)
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]; dest=args.destination.resolve()
if dest.exists(): raise SystemExit('Refusing to overwrite replay directory')
dest.mkdir()
upstream='3a9f41ee85cc369f5b8d7fe6e62ff4e7bf3a9ec8'
def git(*args): return subprocess.check_output(['git','-C',str(root),*args])
scopes=['common','constant','controller','dto','i18n','logger','middleware','model','oauth','pkg','plugins','relay','relaykit','router','service','setting','types','go.mod','go.sum']
available=set(git('ls-tree','--name-only',upstream).decode().splitlines())
with tarfile.open(fileobj=io.BytesIO(git('archive',upstream,*[p for p in scopes if p in available]))) as tar: tar.extractall(dest,filter='data')
files=['constant/seedance_channel.go','model/seedance_plugin_configuration.go','model/channel_asset_credential_storage.go','model/channel_model_mapping.go','model/model_mapping.go','model/seedance_configuration_storage_test.go']+[str(p.relative_to(root)) for p in sorted((root/'pkg/jsplugin').glob('seedance*.go'))]
changes=[]
for name in files:
 p=dest/name
 if p.exists(): raise RuntimeError('Unexpected upstream extension path '+name)
 p.parent.mkdir(exist_ok=True,parents=True);p.write_bytes((root/name).read_bytes());changes.append({'path':name,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()})
# Reuse the exact production statements at equivalent upstream transaction
# points. No icon, task-history, asset-proxy or image-relay implementation is
# needed for the administrative storage contract.
def edit(name,old,new):
 p=dest/name;s=p.read_text();assert s.count(old)==1,(name,old);p.write_text(s.replace(old,new))
production=(root/'model/task_plugin.go').read_text()
lock_plugin='\t\tif err := lockSeedancePluginConfiguration(tx, plugin.Key); err != nil {\n\t\t\treturn err\n\t\t}\n'
lock_key=lock_plugin.replace('plugin.Key','key')
existing='\t\t\tif existing.Active && plugin.Enabled {\n\t\t\t\tif err = validateSeedancePluginConfigurationActivation(tx, &existing); err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n\t\t\t}\n'
first='\t\tif plugin.Active && plugin.Enabled {\n\t\t\tif err = validateSeedancePluginConfigurationActivation(tx, plugin); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n\t\t}\n'
activate='\t\tif err := validateSeedancePluginConfigurationActivation(tx, &target); err != nil {\n\t\t\treturn err\n\t\t}\n'
enable='\tif key == jsplugin.SeedancePluginKey {\n\t\treturn setSeedancePluginConfigurationEnabled(key, enabled)\n\t}\n'
delete='\t\tif err := validateSeedancePluginConfigurationDeletion(tx, &plugin); err != nil {\n\t\t\treturn err\n\t\t}\n'
for block in [lock_plugin,lock_key,existing,first,activate,enable,delete]: assert block in production
name='model/task_plugin.go';before=(dest/name).read_text()
edit(name,'"github.com/QuantumNous/new-api/constant"','"github.com/QuantumNous/new-api/constant"\n "github.com/QuantumNous/new-api/pkg/jsplugin"')
anchor='func SaveTaskPlugin(plugin *TaskPlugin) error {\n\treturn DB.Transaction(func(tx *gorm.DB) error {\n';edit(name,anchor,anchor+lock_plugin)
anchor='\t\t\tif err = tx.Model(&existing).Updates(';edit(name,anchor,existing+anchor)
anchor='\t\tplugin.Active = count == 0\n';edit(name,anchor,anchor+first)
anchor='func ActivateTaskPlugin(key, version string) error {\n\treturn DB.Transaction(func(tx *gorm.DB) error {\n';edit(name,anchor,anchor+lock_key)
anchor='\t\tif err := tx.Model(&TaskPlugin{}).Where(&TaskPlugin{Key: key}).Update("active", false).Error;';edit(name,anchor,activate+anchor)
anchor='func SetTaskPluginEnabled(key string, enabled bool) error {\n';edit(name,anchor,anchor+enable)
anchor='func DeleteTaskPluginVersion(key, version string) (TaskPluginDeleteResult, error) {\n\tresult := TaskPluginDeleteResult{}\n\terr := DB.Transaction(func(tx *gorm.DB) error {\n';edit(name,anchor,anchor+lock_key)
anchor='\t\tresult.DeletedActive = plugin.Active\n';edit(name,anchor,anchor+delete)
channel='model/channel.go';channel_before=(dest/channel).read_text();field=next(line for line in (root/channel).read_text().splitlines(True) if 'SeedancePluginVersion ' in line)
edit(channel,'type Channel struct {\n','type Channel struct {\n'+field)
subprocess.run(['gofmt','-w',str(dest/name),str(dest/channel)],check=True)
patch=''.join(difflib.unified_diff(before.splitlines(True),(dest/name).read_text().splitlines(True),fromfile='a/'+name,tofile='b/'+name))+''.join(difflib.unified_diff(channel_before.splitlines(True),(dest/channel).read_text().splitlines(True),fromfile='a/'+channel,tofile='b/'+channel))
(dest/'native-hooks.patch').write_text(patch)
(dest/'replay-manifest.json').write_text(json.dumps({'upstream':upstream,'extra_files':changes,'native_hooks_sha256':hashlib.sha256(patch.encode()).hexdigest(),'boundary':'Channel configuration read/save validation and native TaskPlugin save/activate/enable/delete promotion. Task execution replay is separate.'},indent=2)+'\n')
print(dest)
