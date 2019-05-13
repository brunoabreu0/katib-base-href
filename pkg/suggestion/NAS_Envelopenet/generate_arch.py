import copy
import re
import math
from collections import Counter

class NACAlg:
    def __init__(self, config, initcell):
        self.config = config
        self.max_filter_prune = int(self.config["max_filter_prune"])
        self.envelopecell = self.config["envelopecell"]
        self.init_cell = initcell
        self.layers_per_stage = self.config["layers_per_stage"]
        self.max_layers_per_stage = self.config["max_layers_per_stage"]
        self.stages = self.config["stages"]
        self.parameter_limits = self.config["parameter_limits"]
        self.construction = self.config["construction"]
        self.branch = {"1":0,"3":1,"5":2,"3sep":3,"5sep":4,"7sep":5}
        self.map = {0:"1",1:"3",2:"5",3:"3sep",4:"5sep",5:"7sep"}

    
    def generate(self):
        """ Generate a network arch based on network config params: Used for
        the initial network
        """
        return self.gen_envelopenet_bystages()

    def construct(self, arch, samples):
        """ Construct a new network based on current arch, metrics from last 
        run and construction config params
        """
        self.arch = arch
        return self.construct_envelopenet_bystages(samples)

    def construct_envelopenet_bystages(self, samples):
        arch = {"type":"macro","network":[]}
        print("Constructing")
        offset_count = [0]
        stages = []
        stage = []
        stagecellnames = {}
        ssidx = {}
        lidx = 1
        ssidx[0] = 1
        stagenum = 0
        for layer in self.arch["network"]:
            if 'widener' in layer:
                lidx += 1
                stages.append(stage)
                stage = []
                stagenum += 1
                ssidx[stagenum] = lidx
            else:
                for branch in layer["filters"]:
                    cellname = 'Cell' + str(lidx) + "/" + branch
                    if stagenum not in stagecellnames:
                        stagecellnames[stagenum] = []
                    stagecellnames[stagenum].append(cellname)
                stage.append(layer)
                lidx += 1
        stages.append(stage)
        stagenum = 0
        narch = []
        for stage in stages:
            if int(self.construction[stagenum]) and len(
                    stage) <= int(self.max_layers_per_stage[stagenum]):

                prune = self.select_prunable(
                    stagecellnames[stagenum], samples)
                nstage = self.prune_filters(ssidx[stagenum], stage, prune)
                nstage = self.add_cell(nstage)
                offset_count += [offset_count[-1]] * len(nstage)
                offset_count[-1] += 1
            else:
                nstage = copy.deepcopy(stage)
                offset_count += [offset_count[-1]] * len(nstage)
            self.set_outputs(nstage, stagenum)
            if stagenum != len(stages) - 1:
                nstage = self.add_widener(nstage)
                offset_count.append(offset_count[-1])
            narch += (nstage)
            stagenum += 1
        
        offset_count = offset_count[1:]
        arch["network"] = narch
        narch = self.insert_skip(arch, samples, offset_count, dense_connect=False)
        return narch

    def remove_logging(self, line):
        line = re.sub(r"\d\d\d\d.*ops.cc:79\] ", "", line)
        return line

         
    def get_samples(self, samples, filter_string='MeanSSS'):
        filtered_log = [line for line in samples if line.startswith(filter_string)]
        return filtered_log

    def get_filter_sample(self, sample):
        fields = sample.split(":")
        filt = fields[1]
        value = float(fields[2].split(']')[0].lstrip('['))
        return filt, value

    def set_outputs(self, stage, stagenum):
        init = self.init_cell
        sinit = sorted(init.keys())
        """ Input channels = output of last layer (conv) in the init """
        for layer in sinit:
            for branch in init[layer]:
                if "outputs" in init[layer][branch]:
                    inputchannels = init[layer][branch]["outputs"]
        width = math.pow(2, stagenum) * inputchannels
        if self.parameter_limits[stagenum]:
            """ Parameter limiting: Calculate output of the internal filters such that
            overall  params is maintained constant
            """
            layers = float(len(stage))

            outputs = int((width / (layers - 2.0)) *
                          (math.pow(layers - 1.0, 0.5) - 1))
            
        lidx = 0
        for layer in stage:
            if "widener" in layer:
                lidx += 1
                continue
            if lidx == len(stage) - \
                    1 or self.parameter_limits[stagenum] is False:
                layer["outputs"] = int(width)
            elif "filters" in layer:
                layer["outputs"] = outputs
            lidx += 1

    def select_prunable(self, stagecellnames, samples):
        measurements = {}
        for sample in samples:
            if sample == '':
                continue
            sample = self.remove_logging(sample)
            filt, value = self.get_filter_sample(sample)

            """ Prune only filters in this stage """
            if filt not in stagecellnames:
                continue

            if filt not in measurements:
                measurements[filt] = []
            measurements[filt].append(value)

        metrics = {}
        for filt in measurements:
            metrics[filt] = measurements[filt][-1]

        smetrics = sorted(metrics, key=metrics.get, reverse=False)
        """ Count number of cells in each layer """
        cellcount = {}
        for cellbr in metrics:
            cellidx = cellbr.split("/")[0].lstrip("Cell")
            if cellidx not in cellcount:
                cellcount[cellidx] = 0
            cellcount[cellidx] += 1

        """ Make sure we do not prune all cells in one layer """
        prunedcount = {}
        prune = []
        for smetric in smetrics:
            prunecellidx = smetric.split("/")[0].lstrip("Cell")
            if prunecellidx not in prunedcount:
                prunedcount[prunecellidx] = 0
            if prunedcount[prunecellidx] + 1 < cellcount[prunecellidx]:

                prune.append(smetric)
                prunedcount[prunecellidx] += 1
                """ Limit number of pruned cells to min of threshold * number of 
                filters in stage and maxfilter prune """
                threshold = (1.0 / 3.0)
                prunecount = min(self.max_filter_prune, int(
                    threshold * float(len(stagecellnames))))
                if len(prune) >= prunecount:
                    break
        if not prune:
            print("Error: No cells to prune")
            exit(-1)
        return prune

    def prune_filters(self, ssidx, stage, prune):
        """ Generate a  pruned network without the wideners """
        narch = []
        lidx = 0
        nfilterlayers = 0
        for layer in stage:
            if 'widener' in layer:
                lidx += 1
                continue
            narch.append(copy.deepcopy(stage[lidx]))
            for filt in stage[lidx]["filters"]:
                fidx = self.branch.get(filt)
                for prn in prune:
                    prunecidx = prn.split("/")[0].lstrip("Cell")
                    prunef = prn.split("/")[1]
                    prunefidx=self.branch.get(prunef)
                    if ssidx + lidx == int(prunecidx) and \
                        fidx == prunefidx:
                        narch[-1]["filters"].remove(self.map.get(prunefidx))
            print("Narc: " + str(narch[-1]))
            nfilterlayers += 1
            lidx += 1
        return narch

    def add_cell(self, narch):
        narch.append({"filters": self.envelopecell})
        return narch

    def add_widener(self, narch):
        narch.append({"widener": {}})
        return narch

    def group_by_layer(self, samples):
        stats = {}
        for sample in samples[::-1]:
            source_node = int(re.search(r'.*:source-(\d+)dest-(\d+).*', sample).group(1))
            dest_node = int(re.search(r'.*:source-(\d+)dest-(\d+).*', sample).group(2))
            if dest_node not in stats.keys():
                stats[dest_node] = {}
            if source_node not in stats[dest_node].keys():
                stats[dest_node][source_node] = float(re.search(r'\[(-?\d+\.\d+)\]', sample).group(1))
        return stats


    def insert_skip(self, narch, samples=None, offset_count=None, dense_connect=False):
        new_network = narch['network']
        if "skip" not in self.config or not self.config['skip']:
            return narch

        if dense_connect == True:
            for layer_id, layer in enumerate(narch['network']):
                if "filters" in layer:
                    new_network[layer_id]["inputs"] = []
                    for connections in range(layer_id - 1, 0, -1):
                        new_network[layer_id]["inputs"].append(connections)
        else:
            threshold = 0.5
            new_connections = []
            scalar_filtered_samples = self.get_samples(samples, filter_string='scalar')
            l2norm_filtered_samples = self.get_samples(samples, filter_string='l2norm')
            scalar_stats = self.group_by_layer(scalar_filtered_samples)
            l2norm_stats = self.group_by_layer(l2norm_filtered_samples)
            """ scalar stats are being used right now """
            for layer_id, layer in enumerate(narch['network'][1:], start=1):
                if "filters" in layer:
                    if offset_count[layer_id] != offset_count[layer_id - 1]:
                        """ New layer has been added at this position, connect densely """
                        new_network[layer_id]["inputs"] = []
                        for connections in range(layer_id - 1, 0, -1):
                            new_network[layer_id]["inputs"].append(connections)
                        new_connections.append(layer_id + 1)
                    else:
                        previous_layer_id = layer_id - offset_count[layer_id]
                        if (previous_layer_id+1) in scalar_stats.keys():
                            number_to_keep = len(scalar_stats[previous_layer_id+1]) - int(threshold * len(scalar_stats[previous_layer_id+1]))
                            print("layer_id = {}, number_to_keep = {}, scalar_stats = {}".format(
                                layer_id, number_to_keep, scalar_stats[previous_layer_id+1]))
                            connections = scalar_stats[previous_layer_id+1]
                            pruned_connections = list(zip(*Counter(connections).most_common(number_to_keep)))[0]

                            updated_pruned_connections = []
                            for connection in pruned_connections:
                                offset = offset_count[connection - 1]
                                while offset_count[offset + connection - 1] != offset:
                                    offset = offset_count[offset + connection - 1]
                                new_index = connection + offset
                                updated_pruned_connections.append(new_index)
                            new_network[layer_id]["inputs"] = updated_pruned_connections+ new_connections

        narch['network'] = new_network
        return narch

    def insert_wideners(self, narch):
        """ Insert wideners,
         Space maxwideners equally with a minimum spacing of self.minwidenerintval
         Last widenerintval may have less layers than others """

        """ widenerintval= nfilterlayers//self.maxwideners """
        widenerintval = len(narch) // self.maxwideners
        if widenerintval < self.minwidenerintval:
            widenerintval = self.minwidenerintval
        nlayer = 1
        insertindices = []
        for layer in narch:
            """ Do not add a widener if it is the last layer """
            if nlayer % widenerintval == 0 and nlayer != len(narch):
                insertindices.append(nlayer)
            nlayer += 1
        idxcnt = 0
        for layeridx in insertindices:
            lidx = layeridx + idxcnt
            """ Adjust insertion indices after inserts """
            narch.insert(lidx, {"widener": {}})
            idxcnt += 1
        return narch

    def gen_envelopenet_bystages(self):
        self.arch = {"type":"macro","network":[]}
        for stageidx in range(int(self.stages)):
            stage = []
            for idx1 in range(int(self.layers_per_stage[stageidx])):
                stage.append({"filters": self.envelopecell})
            self.set_outputs(stage, stageidx)
            if stageidx != int(self.stages) - 1:
                stage = self.add_widener(stage)
            self.arch["network"] += stage
        self.arch = self.insert_skip(self.arch, dense_connect=True)
        return self.arch
