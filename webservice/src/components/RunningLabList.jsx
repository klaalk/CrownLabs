import React, { useState, useEffect } from 'react';
import List from '@material-ui/core/List';
import ListSubheader from '@material-ui/core/ListSubheader';
import CancelOutlinedIcon from '@material-ui/icons/CancelOutlined';
import OpenInBrowserIcon from '@material-ui/icons/OpenInBrowser';
import HourglassEmptyIcon from '@material-ui/icons/HourglassEmpty';
import makeStyles from '@material-ui/core/styles/makeStyles';
import ClickAwayListener from '@material-ui/core/ClickAwayListener';
import SortByAlphaIcon from '@material-ui/icons/SortByAlpha';
import AccessTimeIcon from '@material-ui/icons/AccessTime';
import UserIcon from '@material-ui/icons/Person';
import DesktopIcon from '@material-ui/icons/DesktopWindows';
import TerminalIcon from '@material-ui/icons/ClearAll';
import AllIcon from '@material-ui/icons/GroupWork';
import { utc } from 'moment';
import OrderSelector from './OrderSelector';
import TextSelector from './TextSelector';
import Selector from './Selector';
import { VM_TYPES, VM_STATUS } from '../services/ApiManager';
import ListItem from './ListItem/ListItem';

const useStyles = makeStyles(theme => ({
  root: {
    width: '100%',
    height: '100%',
    maxHeight: '70vh',
    backgroundColor: theme.palette.background.paper,
    position: 'relative',
    overflow: 'auto',
    '& > svg': {
      margin: theme.spacing(2)
    }
  },
  listSubHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    fontSize: '30px',
    padding: theme.spacing(0, 1)
  },
  labColorTag: {
    width: 40,
    height: '100%',
    borderRadius: 5,
    margin: '5px 10px'
  },
  activeLab: {
    backgroundColor: theme.palette.success.main
  },
  loadingLab: {
    backgroundColor: theme.palette.warning.light
  },
  errorLab: {
    backgroundColor: theme.palette.error.light
  },
  rotating: {
    animation: 'rotate 1.5s ease-in-out infinite'
  },
  titleActions: {
    display: 'flex',
    justifyContent: 'end',
    alignItems: 'center'
  }
}));

const studentSelectors = [
  { text: 'Name', icon: <SortByAlphaIcon />, value: 'az' },
  { text: 'Created', icon: <AccessTimeIcon />, value: 'time' }
];
const adminSelectors = [
  ...studentSelectors,
  {
    text: 'User',
    icon: <UserIcon />,
    value: 'user'
  }
];

export const ALL_VM_TYPES = '';
export const vmTypeSelectors = [
  { text: 'All', icon: <AllIcon />, value: ALL_VM_TYPES },
  { text: 'GUI enabled', icon: <DesktopIcon />, value: VM_TYPES.GUI },
  { text: 'CLI only', icon: <TerminalIcon />, value: VM_TYPES.CLI }
];

const getLabCodeFromName = name => /-([0-9]{1,4})$/.exec(name)[1];

const RunningLabList = props => {
  const { labList, stop, connect, title, isStudentView } = props;

  const classes = useStyles();
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const [textMatch, setTextMatch] = useState('');
  const [vmType, setVmType] = useState(() => {
    const prevVmType = JSON.parse(localStorage.getItem(`vmType`));
    return prevVmType || ALL_VM_TYPES;
  });

  const [orderData, setOrderData] = useState(() => {
    const prevOrderData = JSON.parse(
      localStorage.getItem(`orderData-${title}-${isStudentView}`)
    );
    return prevOrderData || { isDirUp: true, order: 'az' };
  });

  useEffect(() => {
    localStorage.setItem(
      `orderData-${title}-${isStudentView}`,
      JSON.stringify(orderData)
    );
  }, [orderData]);

  useEffect(() => {
    localStorage.setItem(`vmType`, JSON.stringify(vmType));
  }, [vmType]);

  return (
    <ClickAwayListener
      onClickAway={() => {
        setSelectedIndex(-1);
      }}
    >
      <List
        className={classes.root}
        subheader={
          <ListSubheader className={classes.listSubHeader}>
            <div>{title}</div>
            <div className={classes.titleActions}>
              <OrderSelector
                selectors={isStudentView ? studentSelectors : adminSelectors}
                setOrderData={setOrderData}
                orderData={orderData}
              />
              <TextSelector value={textMatch} setValue={setTextMatch} />
              <Selector
                selectors={vmTypeSelectors}
                value={vmType}
                setValue={setVmType}
              />
            </div>
          </ListSubheader>
        }
      >
        {labList
          .filter(({ type }) => {
            if (vmType === ALL_VM_TYPES) return true;
            return type === vmType;
          })
          .filter(({ labName, ip, description, studentId }) => {
            if (textMatch !== '') {
              const labCode = getLabCodeFromName(labName);
              const textMatchLower = textMatch.toLowerCase();
              // not using regex but lowercase and include since it sohuld be faster, could be changed easily
              return (
                (description &&
                  description.toLowerCase().includes(textMatchLower)) ||
                labCode.includes(textMatchLower) ||
                ip.includes(textMatchLower) ||
                (studentId &&
                  studentId.toLowerCase().includes(textMatchLower)) ||
                labName.toLowerCase().includes(textMatchLower)
              );
            }
            return true;
          })
          .sort((a, b) => {
            let sortResult = 1;
            const { order, isDirUp } = orderData;
            if (order === 'time')
              sortResult = utc(b.creationTime).diff(a.creationTime, 's');
            else if (order === 'user') {
              sortResult =
                a.studentId && b.studentId
                  ? a.studentId.localeCompare(b.studentId)
                  : a.labName.localeCompare(b.labName);
            } else
              sortResult =
                a.description && b.description
                  ? a.description.localeCompare(b.description)
                  : a.labName.localeCompare(b.labName);

            return isDirUp ? sortResult : -sortResult;
          })
          .map(
            (
              {
                labName,
                status,
                ip,
                creationTime,
                description,
                studentId,
                type
              },
              i
            ) => {
              const labCode = getLabCodeFromName(labName);
              const statusClassName =
                status === VM_STATUS.LOADING
                  ? classes.loadingLab
                  : status === VM_STATUS.READY
                  ? classes.activeLab
                  : classes.errorLab;

              const instanceFields = {
                User: studentId,
                Created: utc(creationTime).local().format('DD/MM/YY HH:mm:ss'),
                IP: ip
              };

              const instanceIcons = [
                {
                  color: 'error',
                  condition: selectedIndex === i && stop,
                  onClick: e => {
                    stop(labName);
                    setSelectedIndex(-1);
                    e.stopPropagation(); // avoid triggering onClick on ListItem
                  },
                  title: 'Stop VM',
                  icon: CancelOutlinedIcon
                },
                {
                  condition:
                    type === VM_TYPES.GUI &&
                    selectedIndex === i &&
                    status === 1,
                  title: 'Connect VM',
                  color: 'info',
                  onClick: e => {
                    connect(labName);
                    setSelectedIndex(-1);
                    e.stopPropagation(); // avoid triggering onClick on ListItem
                  },
                  icon: OpenInBrowserIcon
                },
                {
                  condition: status === 0,
                  title: 'Loading VM',
                  color: 'warning',
                  icon: HourglassEmptyIcon,
                  onClick: () => {},
                  iconClassName: classes.rotating
                }
              ];

              return (
                <ListItem
                  key={labName}
                  primary={
                    description
                      ? `${description} - ${labCode}`
                      : `${labName.charAt(0).toUpperCase()}${labName
                          .slice(1)
                          .replace(/-/g, ' ')}`
                  }
                  fields={instanceFields}
                  icons={instanceIcons}
                  onClick={() => {
                    setSelectedIndex(i);
                  }}
                  type={type}
                  isSelected={selectedIndex === i}
                  showType={vmType === ALL_VM_TYPES}
                  vmTypeSelectors={vmTypeSelectors}
                  customInfo={
                    <div
                      className={`${classes.labColorTag} ${statusClassName}`}
                    >
                      &nbsp;
                    </div>
                  }
                />
              );
            }
          )}
      </List>
    </ClickAwayListener>
  );
};

export default RunningLabList;
