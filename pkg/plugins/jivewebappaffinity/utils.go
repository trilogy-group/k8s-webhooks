package jivewebappaffinity

import (
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"strconv"
)

func resetVarsFromDefaults() {
	maximumHpaReplicas = defaultMaximumHpaReplicas
	hpaName = defaultHpaName
	podLabelForAffinity = defaultPodLabelForAffinity
	topologyKey = defaultTopologyKey
	nsLabelSelStr = defaultNsLabelSelStr
	hpaLabelSelStr = defaultHpaLabelSelStr
	nsPrefix = defaultNsPrefix
}

func setVarsFromYAMLString(yamlString string) {
	var conf map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlString), &conf); err != nil {
		klog.Errorf("Can't parse YAML with config: %v - error: %v", yamlString, err)
		return
	}

	if val, found := conf["maximumHpaReplicas"]; found {
		ival, ok := val.(int)
		if !ok {
			if sval, ok := val.(string); ok {
				if ival, err := strconv.Atoi(sval); err == nil {
					maximumHpaReplicas = ival
				}
			} else {
				maximumHpaReplicas = defaultMaximumHpaReplicas
			}
		} else {
			maximumHpaReplicas = ival
		}
	}
	if val, ok := conf["hpaName"]; ok {
		hpaName, ok = val.(string)
		if !ok {
			hpaName = defaultHpaName
		}
	}
	if val, ok := conf["podLabelForAffinity"]; ok {
		podLabelForAffinity, ok = val.(string)
		if !ok {
			podLabelForAffinity = defaultPodLabelForAffinity
		}
	}
	if val, ok := conf["topologyKey"]; ok {
		topologyKey, ok = val.(string)
		if !ok {
			topologyKey = defaultTopologyKey
		}
	}
	if val, ok := conf["nsLabelSelStr"]; ok {
		nsLabelSelStr, ok = val.(string)
		if !ok {
			nsLabelSelStr = defaultNsLabelSelStr
		}
	}
	if val, ok := conf["hpaLabelSelStr"]; ok {
		hpaLabelSelStr, ok = val.(string)
		if !ok {
			hpaLabelSelStr = defaultHpaLabelSelStr
		}
	}
	if val, ok := conf["nsPrefix"]; ok {
		nsPrefix, ok = val.(string)
		if !ok {
			nsPrefix = defaultNsPrefix
		}
	}
}

func onConfigMapUpdate(old interface{}, new interface{}) {
	resetVarsFromDefaults()
	if cm, ok := new.(*corev1.ConfigMap); ok {
		if _, ok := cm.Data[configMapKey]; !ok {
			return
		}
		setVarsFromYAMLString(cm.Data[configMapKey])
	}

	var err error
	if nsLabelSel, err = labels.Parse(nsLabelSelStr); err != nil {
		klog.Errorf("Invalid NS labels string: %s: %+v - falling back to default: %s",
			nsLabelSelStr, err, defaultNsLabelSelStr)
		nsLabelSel, _ = labels.Parse(defaultNsLabelSelStr)
	}
	if hpaLabelSel, err = labels.Parse(hpaLabelSelStr); err != nil {
		klog.Errorf("Invalid NS labels string: %s: %+v - falling back to default: %s",
			hpaLabelSelStr, err, defaultHpaLabelSelStr)
		hpaLabelSel, _ = labels.Parse(defaultHpaLabelSelStr)
	}

}
